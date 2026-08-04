package config

import (
	"strings"
	"testing"
	"time"
)

func validConfig() Config {
	return Config{
		Environment:         "development",
		NodeID:              "node-1",
		JWTSecret:           "test-secret-with-at-least-thirty-two-characters",
		TokenTTL:            time.Hour,
		EnableRabbitMQ:      true,
		EnableElasticsearch: true,
		AllowedOrigins:      []string{"http://localhost:8080"},
		MaxUploadBytes:      1024,
	}
}

func TestValidateRejectsWeakJWTSecret(t *testing.T) {
	cfg := validConfig()
	cfg.JWTSecret = "change-me"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "IM_JWT_SECRET") {
		t.Fatalf("expected JWT validation error, got %v", err)
	}
}

func TestValidateRequiresRabbitMQForSearch(t *testing.T) {
	cfg := validConfig()
	cfg.EnableRabbitMQ = false
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "RabbitMQ") {
		t.Fatalf("expected RabbitMQ validation error, got %v", err)
	}
}

func TestValidateRejectsWildcardOrigin(t *testing.T) {
	cfg := validConfig()
	cfg.AllowedOrigins = []string{"*"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("expected wildcard validation error, got %v", err)
	}
}
