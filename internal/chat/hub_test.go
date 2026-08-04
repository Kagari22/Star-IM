package chat

import (
	"net/http/httptest"
	"testing"
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
