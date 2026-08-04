param(
    [switch]$WithInfra
)

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Wait-ForService {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [Parameter(Mandatory = $true)]
        [scriptblock]$Probe,
        [int]$Attempts = 30
    )

    for ($attempt = 1; $attempt -le $Attempts; $attempt++) {
        $success = $false
        try {
            & $Probe *> $null
            $success = $?
        }
        catch {
            $success = $false
        }
        if ($success) {
            Write-Host "$Name is ready."
            return
        }
        Start-Sleep -Seconds 2
    }

    throw "$Name did not become ready within $($Attempts * 2) seconds."
}

. (Join-Path $PSScriptRoot "Set-DevEnv.ps1")

if ($WithInfra) {
    docker info *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "Docker Desktop is not running or its Linux container engine is unavailable. Start Docker Desktop, wait until it reports 'Engine running', then run this command again."
    }

    docker compose up -d
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose failed; the Go server was not started. Review the Docker output above and retry after the infrastructure is healthy."
    }

    Wait-ForService -Name "MySQL" -Probe { docker exec im-chat-mysql mysqladmin ping -h 127.0.0.1 -uroot -p123456 }
    Wait-ForService -Name "Redis" -Probe { docker exec im-chat-redis redis-cli ping }
    Wait-ForService -Name "RabbitMQ" -Probe { docker exec im-chat-rabbitmq rabbitmq-diagnostics -q ping }
    Wait-ForService -Name "MinIO" -Probe { docker exec im-chat-minio mc ready local }
    Wait-ForService -Name "Elasticsearch" -Probe { Invoke-WebRequest -Uri "http://127.0.0.1:19200" -UseBasicParsing -TimeoutSec 2 }

    Get-Content .\db\schema.sql | docker exec -i im-chat-mysql mysql -uroot -p123456
    if ($LASTEXITCODE -ne 0) {
        throw "database schema initialization failed; the Go server was not started."
    }
}

go run .\cmd\server
