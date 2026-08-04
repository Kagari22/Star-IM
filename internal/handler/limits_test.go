package handler

import "testing"

func TestParseBoundedLimit(t *testing.T) {
	if limit, err := parseBoundedLimit("", 100); err != nil || limit != 50 {
		t.Fatalf("expected default 50, got %d, %v", limit, err)
	}
	if _, err := parseBoundedLimit("101", 100); err == nil {
		t.Fatal("expected oversized limit rejection")
	}
	if _, err := parseBoundedLimit("0", 100); err == nil {
		t.Fatal("expected zero limit rejection")
	}
}

func TestAllowedMediaType(t *testing.T) {
	if !allowedMediaType("image/png") {
		t.Fatal("expected PNG to be allowed")
	}
	if allowedMediaType("text/html") {
		t.Fatal("expected HTML to be rejected")
	}
}
