$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

docker exec -i im-chat-mysql mysql -uroot -p123456 -e "DROP DATABASE IF EXISTS im_chat; CREATE DATABASE im_chat DEFAULT CHARACTER SET utf8mb4 DEFAULT COLLATE utf8mb4_unicode_ci;"
Get-Content .\db\schema.sql | docker exec -i im-chat-mysql mysql -uroot -p123456

Write-Host "Database reset complete."
