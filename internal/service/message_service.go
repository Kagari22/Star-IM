package service

import (
	"context"
	"errors"
	"io"
	"log"
	"mime"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/repository"
	"IM_Chat_System/internal/storage"
	"IM_Chat_System/internal/unread"
)

type MessageService struct {
	users    repository.UserRepository
	messages repository.MessageRepository
	unread   unread.Store
	uploader storage.Uploader
}

// 构造 MessageService, 注入用户仓储、消息仓储、未读数存储和文件上传器
func NewMessageService(users repository.UserRepository, messages repository.MessageRepository, unreadStore unread.Store, uploader storage.Uploader) *MessageService {
	if uploader == nil {
		uploader = storage.NoopUploader{}
	}
	return &MessageService{
		users:    users,
		messages: messages,
		unread:   unreadStore,
		uploader: uploader,
	}
}

// 获取除当前用户外的用户列表, 并补充每个会话的未读消息数
// MySQL 查询联系人
// → 提取所有对方用户 ID
// → Redis 批量查询会话未读数
// → 写入 User.UnreadCount
func (s *MessageService) ListUsers(ctx context.Context, currentUserID int64) ([]model.User, error) {
	users, err := s.users.List(ctx, currentUserID)
	if err != nil {
		return nil, err
	}
	if s.unread == nil || len(users) == 0 {
		return users, nil
	}

	peerIDs := make([]int64, 0, len(users))
	for _, user := range users {
		peerIDs = append(peerIDs, user.ID)
	}
	counts, err := s.unread.GetConversationCounts(ctx, currentUserID, peerIDs)
	if err != nil {
		return nil, err
	}
	for i := range users {
		users[i].UnreadCount = counts[users[i].ID]
	}
	return users, nil
}

// 查询当前登录用户信息
func (s *MessageService) GetMe(ctx context.Context, userID int64) (model.User, error) {
	user, ok, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return model.User{}, err
	}
	if !ok {
		return model.User{}, errors.New("user not found")
	}
	user.PasswordHash = ""
	return user, nil
}

// 保存文本消息并生成异步事件
// 去除消息首尾空格
// → 校验接收者和消息内容
// → 检查接收者是否存在
// → MySQL 事务保存消息和 Outbox 事件
// → Redis 增加接收者未读数
// → 返回消息
func (s *MessageService) SaveText(ctx context.Context, fromUserID, toUserID int64, content string) (model.Message, error) {
	content = strings.TrimSpace(content)
	if toUserID <= 0 || content == "" {
		return model.Message{}, errors.New("to and content are required")
	}

	if _, ok, err := s.users.GetByID(ctx, toUserID); err != nil {
		return model.Message{}, err
	} else if !ok {
		return model.Message{}, errors.New("receiver not found")
	}

	message, err := s.messages.SaveAndEnqueue(ctx, model.Message{
		FromUserID:  fromUserID,
		ToUserID:    toUserID,
		ContentType: "text",
		Content:     content,
	})
	if err != nil {
		return model.Message{}, err
	}
	if s.unread != nil {
		if _, err := s.unread.Increment(ctx, toUserID, fromUserID, message.ID); err != nil {
			log.Printf("increment unread for message %d: %v", message.ID, err)
		}
	}
	return message, nil
}

// 保存图片或文件消息
// 校验接收者
// → 清理文件名
// → 推断文件 MIME 类型
// → 生成对象存储 Key
// → 上传 MinIO
// → 判断消息类型是 image 还是 file
// → MySQL 保存消息和 Outbox 事件
// → Redis 增加未读数
// → 生成短时预签名 URL
// → 返回媒体消息
func (s *MessageService) SaveMedia(ctx context.Context, fromUserID, toUserID int64, fileName string, size int64, contentType string, reader io.Reader) (model.Message, error) {
	if toUserID <= 0 {
		return model.Message{}, errors.New("to_user_id is required")
	}
	if _, ok, err := s.users.GetByID(ctx, toUserID); err != nil {
		return model.Message{}, err
	} else if !ok {
		return model.Message{}, errors.New("receiver not found")
	}

	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "" {
		fileName = "file"
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(fileName))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	objectName := buildObjectName(fromUserID, toUserID, fileName)
	object, err := s.uploader.Upload(ctx, objectName, reader, size, contentType)
	if err != nil {
		return model.Message{}, err
	}

	messageType := "file"
	if strings.HasPrefix(contentType, "image/") {
		messageType = "image"
	}

	message, err := s.messages.SaveAndEnqueue(ctx, model.Message{
		FromUserID:  fromUserID,
		ToUserID:    toUserID,
		ContentType: messageType,
		Content:     fileName,
		ObjectKey:   object.Key,
		ObjectURL:   object.URL,
		FileName:    fileName,
		FileSize:    object.Size,
	})
	if err != nil {
		return model.Message{}, err
	}
	if s.unread != nil {
		if _, err := s.unread.Increment(ctx, toUserID, fromUserID, message.ID); err != nil {
			log.Printf("increment unread for message %d: %v", message.ID, err)
		}
	}
	enriched, err := s.EnrichMessage(ctx, message)
	if err != nil {
		log.Printf("presign media for message %d: %v", message.ID, err)
		return message, nil
	}
	return enriched, nil
}

// 获取当前用户与指定用户之间的聊天历史
// 校验 peerID
// → 规范化 limit
// → 查询双向聊天记录
// → 清除已读范围内的 Redis 未读数
// → 为媒体消息补充预签名 URL
// → 返回消息列表
func (s *MessageService) Conversation(ctx context.Context, userID, peerID, afterID int64, limit int) ([]model.Message, error) {
	if peerID <= 0 {
		return nil, errors.New("peer_id is required")
	}
	limit = normalizeLimit(limit, 100)
	messages, err := s.messages.ListConversation(ctx, userID, peerID, afterID, limit)
	if err != nil {
		return nil, err
	}
	if s.unread != nil {
		throughID := afterID
		if len(messages) > 0 {
			throughID = messages[len(messages)-1].ID
		}
		if err := s.unread.ClearConversation(ctx, userID, peerID, throughID); err != nil {
			return nil, err
		}
	}
	return s.EnrichMessages(ctx, messages)
}

// 获取当前用户离线期间收到的消息
// 规范化 limit
// → 查询 to_user_id = 当前用户且 id > afterID 的消息
// → 补充媒体预签名 URL
// → 返回离线消息
func (s *MessageService) Offline(ctx context.Context, userID, afterID int64, limit int) ([]model.Message, error) {
	limit = normalizeLimit(limit, 100)
	messages, err := s.messages.ListOffline(ctx, userID, afterID, limit)
	if err != nil {
		return nil, err
	}
	return s.EnrichMessages(ctx, messages)
}

// 为单条图片或文件消息补充最新的 MinIO 预签名访问 URL
// 如果消息没有 ObjectKey, 说明不是媒体消息, 直接返回原消息
func (s *MessageService) EnrichMessage(ctx context.Context, message model.Message) (model.Message, error) {
	if message.ObjectKey == "" || s.uploader == nil {
		return message, nil
	}
	url, err := s.uploader.PresignGet(ctx, message.ObjectKey)
	if err != nil {
		return model.Message{}, err
	}
	message.ObjectURL = url
	return message, nil
}

// 批量调用 EnrichMessage, 为消息列表中的所有媒体消息生成访问 URL
// 搜索结果、聊天历史和离线消息都会使用该函数
func (s *MessageService) EnrichMessages(ctx context.Context, messages []model.Message) ([]model.Message, error) {
	for i := range messages {
		message, err := s.EnrichMessage(ctx, messages[i])
		if err != nil {
			return nil, err
		}
		messages[i] = message
	}
	return messages, nil
}

// 统一处理分页数量
func normalizeLimit(limit, maximum int) int {
	if limit <= 0 {
		return 50
	}
	if limit > maximum {
		return maximum
	}
	return limit
}

// 生成 MinIO 对象存储路径
func buildObjectName(fromUserID, toUserID int64, fileName string) string {
	safeName := strings.ReplaceAll(fileName, " ", "_")
	return "chat/" + strconv.FormatInt(fromUserID, 10) + "/" + strconv.FormatInt(toUserID, 10) + "/" + strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + safeName
}
