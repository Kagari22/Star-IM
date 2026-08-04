package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"IM_Chat_System/internal/httpx"
	"IM_Chat_System/internal/service"
)

type MediaHandler struct {
	messages *service.MessageService
	maxBytes int64
}

func NewMediaHandler(messages *service.MessageService, maxBytes int64) *MediaHandler {
	return &MediaHandler{messages: messages, maxBytes: maxBytes}
}

/*
JWT 身份校验
→ 限制请求体大小
→ 解析 multipart/form-data
→ 获取接收者和文件
→ 校验文件大小
→ 读取文件前 512 字节识别真实 MIME 类型
→ 白名单校验
→ 上传至 MinIO、保存消息（Service 层）
→ 返回创建成功的消息
*/
func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	// 限制整个 HTTP 请求体的最大体积。
	// 这样可以在读取文件前阻止超大请求, 防止有人通过超大文件占满服务内存、磁盘或网络带宽。
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes)
	if err := r.ParseMultipartForm(h.maxBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	// 从表单读取 to_user_id -> 文件要发送给谁
	toUserID, _ := strconv.ParseInt(r.FormValue("to_user_id"), 10, 64)
	// file: 上传文件的数据流
	file, header, err := r.FormFile("file") 
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > h.maxBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
		return
	}

	peek := make([]byte, 512)
	n, _ := io.ReadFull(file, peek)
	contentType := http.DetectContentType(peek[:n]) // 检测文件或数据的 MIME 类型
	if !allowedMediaType(contentType) { // 只允许白名单类型, 如 JPEG、PNG、GIF、WebP、PDF、TXT
		httpx.WriteError(w, http.StatusUnsupportedMediaType, "unsupported file type")
		return
	}
	reader := io.MultiReader(bytes.NewReader(peek[:n]), file)

	// 交给 Service 层处理具体业务：校验接收者是否存在、上传对象存储、生成预签名访问地址、创建文件/图片消息，并写入数据库和 Outbox 事件
	message, err := h.messages.SaveMedia(r.Context(), claims.UserID, toUserID, header.Filename, header.Size, contentType, reader)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"message": message})
}

// 判断文件的 MIME 类型是否在允许上传的白名单中
func allowedMediaType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch mediaType {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "application/pdf", "text/plain":
		return true
	default:
		return false
	}
}
