#!/bin/sh
# Nightly logical backups of PostgreSQL, run by the `backup` service in
# deploy/compose.yml.
#
# pg_dump takes a consistent snapshot (one transaction, MVCC) without
# blocking the application. "-Fc" is the custom format: compressed, and
# pg_restore can restore single tables from it.
#
# Restore with deploy/restore.sh. A backup you have never restored is a
# hope, not a backup: test restores regularly.
set -eu

interval="${BACKUP_INTERVAL_SECONDS:-86400}"
keep_days="${BACKUP_KEEP_DAYS:-7}"

while true; do
	stamp=$(date -u +%Y%m%dT%H%M%SZ)
	file="/backups/auction-$stamp.dump"

	# Write to a temporary name and rename at the end, so a crash halfway
	# never leaves a truncated file that looks like a good backup.
	if pg_dump -Fc -f "$file.partial"; then
		mv "$file.partial" "$file"
		echo "backup written: $file ($(du -h "$file" | cut -f1))"
	else
		rm -f "$file.partial"
		echo "backup FAILED at $stamp" >&2
	fi

	# Rotation: delete dumps older than keep_days.
	find /backups -name 'auction-*.dump' -mtime +"$keep_days" -print -delete

	sleep "$interval"
done
