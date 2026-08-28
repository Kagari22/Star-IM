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

function Get-MissingFeatureCount {
    $query = @"
SELECT COUNT(*)
FROM (
  SELECT 'chat_groups.all_muted' AS feature
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'chat_groups' AND COLUMN_NAME = 'all_muted'
  )
  UNION ALL
  SELECT 'chat_groups.invite_code'
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'chat_groups' AND COLUMN_NAME = 'invite_code'
  )
  UNION ALL
  SELECT 'messages.reply_to_id'
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'messages' AND COLUMN_NAME = 'reply_to_id'
  )
  UNION ALL
  SELECT 'messages.edited_at'
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'messages' AND COLUMN_NAME = 'edited_at'
  )
  UNION ALL
  SELECT 'message_favorites'
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'message_favorites'
  )
  UNION ALL
  SELECT 'message_pins'
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'message_pins'
  )
  UNION ALL
  SELECT 'user_message_hides'
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'user_message_hides'
  )
  UNION ALL
  SELECT 'conversation_reads'
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'conversation_reads'
  )
  UNION ALL
  SELECT 'conversation_preferences'
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'conversation_preferences'
  )
  UNION ALL
  SELECT 'group_kick_logs'
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'group_kick_logs'
  )
) AS missing_features;
"@
    $result = docker exec im-chat-mysql mysql -N -s -uroot -p123456 -e $query
    if ($LASTEXITCODE -ne 0) {
        throw "database feature check failed; the Go server was not started."
    }
    return [int]($result | Select-Object -First 1)
}

function HasOutboxEventIndex {
    $result = docker exec im-chat-mysql mysql -N -s -uroot -p123456 -e "SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'outbox_events' AND INDEX_NAME = 'idx_outbox_event_message' AND NON_UNIQUE = 1;"
    if ($LASTEXITCODE -ne 0) {
        throw "outbox index check failed; the Go server was not started."
    }
    return [int]($result | Select-Object -First 1) -gt 0
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

    # schema.sql handles fresh databases. Existing databases need the feature
    # migrations below; 009 also recovers databases where an earlier 008 run
    # stopped after adding columns but before creating every new table.
    $missingFeatures = Get-MissingFeatureCount
    $hasAllMuted = docker exec im-chat-mysql mysql -N -s -uroot -p123456 -e "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'chat_groups' AND COLUMN_NAME = 'all_muted';"
    if ($LASTEXITCODE -ne 0) {
        throw "database migration check failed; the Go server was not started."
    }
    if ($missingFeatures -gt 0 -and [int]($hasAllMuted | Select-Object -First 1) -eq 0) {
        Get-Content .\db\migrations\008_message_conversation_group_features.sql | docker exec -i im-chat-mysql mysql -uroot -p123456 im_chat
        if ($LASTEXITCODE -ne 0) {
            throw "database migration 008 failed; the Go server was not started."
        }
        Write-Host "Database migration 008 applied."
    }
    elseif ($missingFeatures -gt 0) {
        Get-Content .\db\migrations\009_repair_message_conversation_group_features.sql | docker exec -i im-chat-mysql mysql -uroot -p123456 im_chat
        if ($LASTEXITCODE -ne 0) {
            throw "database repair migration 009 failed; the Go server was not started."
        }
        Write-Host "Database repair migration 009 applied."
    }

    if (-not (HasOutboxEventIndex)) {
        Get-Content .\db\migrations\010_allow_repeated_message_updates.sql | docker exec -i im-chat-mysql mysql -uroot -p123456 im_chat
        if ($LASTEXITCODE -ne 0) {
            throw "database migration 010 failed; the Go server was not started."
        }
        Write-Host "Database migration 010 applied."
    }

    $hasRedPackets = docker exec im-chat-mysql mysql -N -s -uroot -p123456 -e "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = 'im_chat' AND TABLE_NAME = 'red_packets';"
    if ($LASTEXITCODE -ne 0) {
        throw "database red-packet check failed; the Go server was not started."
    }
    if ([int]($hasRedPackets | Select-Object -First 1) -eq 0) {
        Get-Content .\db\migrations\011_red_packet.sql | docker exec -i im-chat-mysql mysql -uroot -p123456 im_chat
        if ($LASTEXITCODE -ne 0) {
            throw "database migration 011 failed; the Go server was not started."
        }
        Write-Host "Database migration 011 applied."
    }

    if ((Get-MissingFeatureCount) -gt 0) {
        throw "database migration did not install every required message and group feature; the Go server was not started."
    }
}

go run .\cmd\server
