#!/bin/bash
pkill -f "go run ./cmd/server" 2>/dev/null
pkill -f "emplacc-main-api" 2>/dev/null
lsof -ti:8081 | xargs kill -9 2>/dev/null
sleep 1

cd "$(dirname "$0")"

# Загружаем .env построчно (безопасно для URLs и спецсимволов)
set -o allexport
# shellcheck disable=SC1091
source .env
set +o allexport

go run ./cmd/server/main.go >> /tmp/emplacc-api.log 2>&1 &
echo "API запущен (PID $!)"
