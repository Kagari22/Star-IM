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

# AI chat bot (OpenAI-compatible, DeepSeek by default).
# IM_AI_API_KEY is not set here; export it before starting, e.g.:
#   $env:IM_AI_API_KEY = "your-deepseek-api-key"
# Bot nickname deliberately not set here: PowerShell 5.1 misreads UTF-8
# Chinese literals in BOM-less scripts, so the Go default is used instead.
$env:IM_AI_ENABLED = "true"
if (-not $env:IM_AI_BASE_URL) { $env:IM_AI_BASE_URL = "https://api.deepseek.com/v1" }
if (-not $env:IM_AI_MODEL) { $env:IM_AI_MODEL = "deepseek-chat" }
if (-not $env:IM_AI_BOT_USERNAME) { $env:IM_AI_BOT_USERNAME = "ai_assistant" }

# Load local secrets (IM_AI_API_KEY etc.) from scripts/secrets.ps1.
# That file is gitignored so the key never enters version control.
$secretsFile = Join-Path $PSScriptRoot "secrets.ps1"
if (Test-Path $secretsFile) { . $secretsFile }
if ($env:IM_AI_ENABLED -eq "true" -and (-not $env:IM_AI_API_KEY -or $env:IM_AI_API_KEY -like "*paste-your*")) {
    Write-Warning "IM_AI_ENABLED=true but IM_AI_API_KEY is empty or still the placeholder; AI replies will fail with 401. Put the real key in scripts/secrets.ps1."
}

$env:GOCACHE = Join-Path $root ".gocache"
$env:GOMODCACHE = Join-Path $root ".gomodcache"

Write-Host "Development environment variables loaded."
