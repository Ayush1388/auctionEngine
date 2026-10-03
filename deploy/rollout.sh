#!/usr/bin/env bash
# Zero-downtime deploy of a new version.
#
#   IMAGE=ghcr.io/ayush1388/auctionengine:<commit-sha> ./deploy/rollout.sh
#
# 1. Get the new image (pull from the registry, or build locally).
# 2. Run migrations. They must be backward compatible with the version
#    still running (expand -> deploy -> contract), because old and new code
#    share the database for the length of the rollout.
# 3. Replace containers ONE AT A TIME, waiting for each to report healthy
#    before touching the next. While api-1 restarts, Caddy sends everything
#    to api-2, and vice versa. A stopping API drains first (readiness 503,
#    DRAIN_DELAY, then finishes in-flight requests), so nothing is dropped.
set -euo pipefail

cd "$(dirname "$0")"
compose=(docker compose -f compose.yml)

if [[ "${IMAGE:-auctionengine:local}" == "auctionengine:local" ]]; then
	"${compose[@]}" build migrate
else
	"${compose[@]}" pull migrate
fi

echo "==> migrating"
"${compose[@]}" run --rm migrate

wait_healthy() {
	local svc=$1 id status
	id=$("${compose[@]}" ps -q "$svc")
	for _ in $(seq 1 60); do
		status=$(docker inspect --format '{{.State.Health.Status}}' "$id" 2>/dev/null || echo starting)
		if [[ "$status" == "healthy" ]]; then
			echo "    $svc healthy"
			return 0
		fi
		sleep 2
	done
	echo "    $svc did not become healthy; stopping the rollout" >&2
	"${compose[@]}" logs --tail 50 "$svc" >&2
	return 1
}

# Backends first, so new API code never calls an old bidding service that
# lacks a method it needs (deploy providers before consumers).
for svc in biddingsvc bidworker api-1 api-2; do
	echo "==> replacing $svc"
	"${compose[@]}" up -d --no-deps --no-build --force-recreate "$svc"
	wait_healthy "$svc"
done

echo "==> rollout complete"
