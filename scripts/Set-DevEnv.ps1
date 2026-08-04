$root = Split-Path -Parent $PSScriptRoot

$env:IM_ADDR = ":8080"
$env:IM_ENV = "development"
$env:IM_NODE_ID = "node-1"
$env:IM_JWT_SECRET = "development-only-secret-change-before-production-2026"
$env:IM_TOKEN_TTL_HOURS = "168"
$env:IM_ALLOWED_ORIGINS = "http://127.0.0.1:8080,http://localhost:8080,http://127.0.0.1:5173,http://localhost:5173"
$env:IM_MAX_UPLOAD_BYTES = "10485760"

$env:IM_MYSQL_DSN = "root:123456@tcp(127.0.0.1:13306)/im_chat?parseTime=true&charset=utf8mb4&loc=Local"

$env:IM_REDIS_ADDR = "127.0.0.1:16379"
$env:IM_REDIS_PASSWORD = ""
$env:IM_REDIS_DB = "0"

$env:IM_ENABLE_RABBITMQ = "true"
$env:IM_RABBITMQ_URL = "amqp://guest:guest@127.0.0.1:15673/"

$env:IM_ENABLE_MINIO = "true"
$env:IM_MINIO_ENDPOINT = "127.0.0.1:19000"
$env:IM_MINIO_ACCESS_KEY = "minioadmin"
$env:IM_MINIO_SECRET_KEY = "minioadmin"
$env:IM_MINIO_BUCKET = "im-chat"
$env:IM_MINIO_USE_SSL = "false"
$env:IM_MEDIA_URL_TTL_MINUTES = "15"

$env:IM_ENABLE_ELASTICSEARCH = "true"
$env:IM_ELASTICSEARCH_URL = "http://127.0.0.1:19200"
$env:IM_ELASTICSEARCH_INDEX = "messages"

$env:GOCACHE = Join-Path $root ".gocache"
$env:GOMODCACHE = Join-Path $root ".gomodcache"

Write-Host "Development environment variables loaded."
