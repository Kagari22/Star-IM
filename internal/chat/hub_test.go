package chat

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/mq"
	"IM_Chat_System/internal/service"
)

func TestWebsocketTokenUsesSubprotocolOnly(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.test/ws?token=not-accepted", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "im-chat, signed-token")
	if token := websocketToken(req); token != "signed-token" {
		t.Fatalf("expected subprotocol token, got %q", token)
	}

	req.Header.Set("Sec-WebSocket-Protocol", "signed-token")
	if token := websocketToken(req); token != "" {
		t.Fatalf("expected malformed subprotocol to be rejected, got %q", token)
	}
}

func TestOriginAllowlist(t *testing.T) {
	hub := NewHub(nil, "secret", "node-1", nil, nil, nil, []string{"https://app.example.test"})
	req := httptest.NewRequest("GET", "https://app.example.test/ws", nil)
	req.Header.Set("Origin", "https://app.example.test")
	if !hub.isOriginAllowed(req) {
		t.Fatal("expected configured origin to be accepted")
	}
	req.Header.Set("Origin", "https://evil.example.test")
	if hub.isOriginAllowed(req) {
		t.Fatal("expected unconfigured origin to be rejected")
	}
}

type groupRepositoryStub struct {
	members []model.GroupMember
}

func (s *groupRepositoryStub) Create(context.Context, int64, string, []int64) (model.Group, error) {
	return model.Group{}, nil
}

func (s *groupRepositoryStub) ListForUser(context.Context, int64) ([]model.Group, error) {
	return nil, nil
}

func (s *groupRepositoryStub) GetByID(context.Context, int64) (model.Group, bool, error) {
	return model.Group{}, false, nil
}

func (s *groupRepositoryStub) IsMember(ctx context.Context, groupID, userID int64) (model.GroupMember, bool, error) {
	for _, member := range s.members {
		if member.GroupID == groupID && member.UserID == userID {
			return member, true, nil
		}
	}
	return model.GroupMember{}, false, nil
}

func (s *groupRepositoryStub) ListMembers(context.Context, int64) ([]model.GroupMember, error) {
	return s.members, nil
}

func (s *groupRepositoryStub) SearchGroups(context.Context, string, int) ([]model.Group, error) {
	return nil, nil
}

func (s *groupRepositoryStub) Dissolve(context.Context, int64) error { return nil }

func (s *groupRepositoryStub) UpdateAvatarKey(context.Context, int64, string) error { return nil }
func (s *groupRepositoryStub) AddMember(context.Context, int64, int64) error        { return nil }
func (s *groupRepositoryStub) RemoveMember(context.Context, int64, int64) error     { return nil }
func (s *groupRepositoryStub) UpdateAnnouncement(context.Context, int64, int64, string) (model.Group, error) {
	return model.Group{}, nil
}
func (s *groupRepositoryStub) SetAllMuted(context.Context, int64, int64, bool) (model.Group, error) {
	return model.Group{}, nil
}
func (s *groupRepositoryStub) SetMemberRole(context.Context, int64, int64, model.GroupRole) error {
	return nil
}
func (s *groupRepositoryStub) SetMemberMutedUntil(context.Context, int64, int64, *time.Time) error {
	return nil
}

type presenceStub struct {
	online map[int64]string
}

func (s *presenceStub) SetOnline(context.Context, int64, string) error { return nil }

func (s *presenceStub) GetOnlineNode(_ context.Context, userID int64) (string, bool, error) {
	node, ok := s.online[userID]
	return node, ok, nil
}

func (s *presenceStub) SetOffline(context.Context, int64) error { return nil }

func TestDispatchMessageCreatedGroup(t *testing.T) {
	groupID := int64(42)
	groups := &groupRepositoryStub{members: []model.GroupMember{
		{GroupID: groupID, UserID: 1, Role: model.GroupRoleOwner},
		{GroupID: groupID, UserID: 2, Role: model.GroupRoleMember},
		{GroupID: groupID, UserID: 3, Role: model.GroupRoleMember},
	}}
	presence := &presenceStub{online: map[int64]string{
		1: "node-1",
		2: "node-1",
		3: "node-2",
	}}
	messages := service.NewMessageService(nil, groups, nil, nil, nil, nil)
	hub := NewHub(messages, "secret", "node-1", presence, nil, nil, nil)
	hub.clients[2] = &Client{send: make(chan any, 1)}
	hub.clients[3] = &Client{send: make(chan any, 1)}

	err := hub.DispatchMessageCreated(context.Background(), mq.MessageCreatedEvent{
		MessageID:   100,
		FromUserID:  1,
		GroupID:     &groupID,
		ContentType: "text",
		Content:     "hello group",
		CreatedAt:   time.Now(),
	})
	if err != nil {
		t.Fatalf("dispatch group message: %v", err)
	}

	payload := <-hub.clients[2].send
	outgoing, ok := payload.(OutgoingMessage)
	if !ok {
		t.Fatalf("expected OutgoingMessage, got %T", payload)
	}
	if outgoing.Type != "chat" || outgoing.Message.GroupID == nil || *outgoing.Message.GroupID != groupID {
		t.Fatalf("unexpected group payload: %+v", outgoing)
	}

	select {
	case <-hub.clients[3].send:
		t.Fatal("member connected to another node should not receive local delivery")
	default:
	}
}
