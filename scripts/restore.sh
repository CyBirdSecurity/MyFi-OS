#!/usr/bin/env bash
# Restores the MyFi Postgres database (the "db" docker compose service) from
# a backup produced by scripts/backup.sh. Overwrites all current data.
set -euo pipefail

cd "$(dirname "$0")/.."

file="${1:?Usage: scripts/restore.sh <backup-file.sql.gz>}"
if [ ! -f "$file" ]; then
  echo "Backup file not found: $file" >&2
  exit 1
fi

POSTGRES_USER="${POSTGRES_USER:-myfi}"
POSTGRES_DB="${POSTGRES_DB:-myfi}"

echo "WARNING: this will overwrite the current '$POSTGRES_DB' database with the"
echo "contents of $file. All existing data will be lost."
read -r -p "Type 'yes' to continue: " confirm
if [ "$confirm" != "yes" ]; then
  echo "Aborted."
  exit 1
fi

# Stop the app so nothing writes while the data is replaced, and start it again
# however the restore ends.
docker compose stop app
trap 'docker compose start app >/dev/null' EXIT

# Replace the whole schema in one transaction and stop at the first error, so a
# restore either fully succeeds or leaves the current data untouched.
{
  echo "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
  gunzip -c "$file"
} | docker compose exec -T db \
  psql -q -v ON_ERROR_STOP=1 --single-transaction -U "$POSTGRES_USER" "$POSTGRES_DB" >/dev/null

echo "Restore complete."
echo
echo "If your current MYFI_ENCRYPTION_KEY differs from the one in place when this"
echo "backup was taken, Plaid-linked accounts will fail to decrypt — unlink and"
echo "re-link them under Settings > Plaid Connections."
