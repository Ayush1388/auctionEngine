#!/usr/bin/env bash
# Restore a backup made by backup.sh into the running database.
#
#   ./deploy/restore.sh deploy/backups/auction-20261001T020000Z.dump
#
# This REPLACES the current data. The API is stopped first so nothing
# writes while tables are being dropped and recreated, and started again
# afterwards.
set -euo pipefail

dump="${1:?usage: restore.sh <file.dump>}"
cd "$(dirname "$0")"
compose=(docker compose -f compose.yml)

read -r -p "Replace the database with $dump? Type 'restore' to continue: " answer
[[ "$answer" == "restore" ]] || { echo "aborted"; exit 1; }

"${compose[@]}" stop api-1 api-2 biddingsvc bidworker

# --clean --if-exists drops each object before recreating it.
"${compose[@]}" exec -T postgres \
	pg_restore --clean --if-exists --no-owner -U auction -d auction < "$dump"

"${compose[@]}" up -d api-1 api-2 biddingsvc bidworker
echo "restored $dump"
