#!/usr/bin/env bash
# Dumps the MyFi Postgres database (the "db" docker compose service) to a
# timestamped, gzip-compressed file under backups/.
set -euo pipefail

cd "$(dirname "$0")/.."

POSTGRES_USER="${POSTGRES_USER:-myfi}"
POSTGRES_DB="${POSTGRES_DB:-myfi}"

mkdir -p backups
timestamp=$(date +%Y%m%d_%H%M%S)
out="backups/myfi_${timestamp}.sql.gz"

docker compose exec -T db \
  pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" | gzip > "$out"

echo "Backup written to $out"
echo
echo "IMPORTANT: this backup only contains the database. If Plaid is configured,"
echo "also back up your .env file's MYFI_ENCRYPTION_KEY somewhere safe — it is"
echo "required to decrypt Plaid access tokens stored in this dump. Restoring"
echo "under a different key will break bank-sync for every linked account."
