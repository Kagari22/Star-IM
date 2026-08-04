package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment         string
	HTTPAddr            string
	NodeID              string
	JWTSecret           string
	TokenTTL            time.Duration
	MySQLDSN            string
	RedisAddr           string
	RedisPassword       string
	RedisDB             int
	EnableRabbitMQ      bool
	RabbitMQURL         string
	EnableMinIO         bool
	MinIOEndpoint       string
	MinIOAccessKey      string
	MinIOSecretKey      string
	MinIOBucket         string
	MinIOUseSSL         bool
	MediaURLTTL         time.Duration
	EnableElasticsearch bool
	ElasticsearchURL    string
	ElasticsearchIndex  string
	AllowedOrigins      []string
	MaxUploadBytes      int64
}

func Load() Config {
	return Config{
		Environment:         getenv("IM_ENV", "development"),
		HTTPAddr:            getenv("IM_ADDR", ":8080"),
		NodeID:              getenv("IM_NODE_ID", "node-1"),
		JWTSecret:           getenv("IM_JWT_SECRET", "change-me"),
		TokenTTL:            time.Duration(getenvInt("IM_TOKEN_TTL_HOURS", 168)) * time.Hour,
		MySQLDSN:            getenv("IM_MYSQL_DSN", "root:123456@tcp(127.0.0.1:3306)/im_chat?parseTime=true&charset=utf8mb4&loc=Local"),
		RedisAddr:           getenv("IM_REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:       os.Getenv("IM_REDIS_PASSWORD"),
		RedisDB:             getenvInt("IM_REDIS_DB", 0),
		EnableRabbitMQ:      getenvBool("IM_ENABLE_RABBITMQ", true),
		RabbitMQURL:         getenv("IM_RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:5672/"),
		EnableMinIO:         getenvBool("IM_ENABLE_MINIO", true),
		MinIOEndpoint:       getenv("IM_MINIO_ENDPOINT", "127.0.0.1:9000"),
		MinIOAccessKey:      getenv("IM_MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:      getenv("IM_MINIO_SECRET_KEY", "minioadmin"),
		MinIOBucket:         getenv("IM_MINIO_BUCKET", "im-chat"),
		MinIOUseSSL:         getenvBool("IM_MINIO_USE_SSL", false),
		MediaURLTTL:         time.Duration(getenvInt("IM_MEDIA_URL_TTL_MINUTES", 15)) * time.Minute,
		EnableElasticsearch: getenvBool("IM_ENABLE_ELASTICSEARCH", true),
		ElasticsearchURL:    getenv("IM_ELASTICSEARCH_URL", "http://127.0.0.1:9200"),
		ElasticsearchIndex:  getenv("IM_ELASTICSEARCH_INDEX", "messages"),
		AllowedOrigins:      splitCSV(getenv("IM_ALLOWED_ORIGINS", "http://127.0.0.1:8080,http://localhost:8080,http://127.0.0.1:5173,http://localhost:5173")),
		MaxUploadBytes:      int64(getenvInt("IM_MAX_UPLOAD_BYTES", 10<<20)),
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.NodeID) == "" {
		return fmt.Errorf("IM_NODE_ID is required")
	}
	if len(c.JWTSecret) < 32 || c.JWTSecret == "change-me" {
		return fmt.Errorf("IM_JWT_SECRET must be at least 32 characters and must not use the default value")
	}
	if c.TokenTTL <= 0 {
		return fmt.Errorf("IM_TOKEN_TTL_HOURS must be positive")
	}
	if c.MaxUploadBytes <= 0 {
		return fmt.Errorf("IM_MAX_UPLOAD_BYTES must be positive")
	}
	if c.EnableMinIO && c.MediaURLTTL <= 0 {
		return fmt.Errorf("IM_MEDIA_URL_TTL_MINUTES must be positive when MinIO is enabled")
	}
	if c.EnableElasticsearch && !c.EnableRabbitMQ {
		return fmt.Errorf("RabbitMQ must be enabled when Elasticsearch indexing is enabled")
	}
	if len(c.AllowedOrigins) == 0 {
		return fmt.Errorf("IM_ALLOWED_ORIGINS must contain at least one origin")
	}
	for _, origin := range c.AllowedOrigins {
		if origin == "*" {
			return fmt.Errorf("IM_ALLOWED_ORIGINS must not contain wildcard origins")
		}
	}
	if strings.EqualFold(c.Environment, "production") {
		if c.EnableMinIO && (!c.MinIOUseSSL || c.MinIOAccessKey == "minioadmin" || c.MinIOSecretKey == "minioadmin") {
			return fmt.Errorf("production MinIO configuration requires TLS and non-default credentials")
		}
		if c.JWTSecret == "change-me" {
			return fmt.Errorf("production requires a non-default JWT secret")
		}
	}
	return nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "TRUE", "True", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "False", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
