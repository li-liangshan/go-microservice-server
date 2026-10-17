#!/usr/bin/env bash
# scripts/seed_users.sh — populate the `users` table with N synthetic rows.
#
# Usage: ./scripts/seed_users.sh [count]   (default: 10000)
#
# Env overrides (with defaults shown):
#   DB_HOST=127.0.0.1  DB_PORT=3306
#   DB_USER=root       DB_PASS=admin
#   DB_NAME=microservice
#
# Approach: 1000 rows per multi-row INSERT to keep statements readable while
# staying fast (~10 INSERTs for 10000 rows). Names are drawn from 20 first ×
# 20 last combinations, so the dataset has 400 unique display names but each
# row has a unique email via the trailing sequence number.
#
# Does NOT truncate: run `TRUNCATE TABLE users;` first if you want a clean
# dataset of exactly N rows.

set -euo pipefail

COUNT="${1:-10000}"
DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-root}"
DB_PASS="${DB_PASS:-admin}"
DB_NAME="${DB_NAME:-microservice}"
BATCH=1000

FIRST=(alice bob carol david eve frank grace henry ivy jack
       kate leo mia noah olivia peter quinn ryan sophia taylor)
LAST=(smith johnson williams brown jones garcia miller davis rodriguez martinez
      hernandez lopez wilson anderson thomas taylor moore jackson martin lee)

NF=${#FIRST[@]}
NL=${#LAST[@]}
BATCHES=$(( (COUNT + BATCH - 1) / BATCH ))

SQL=$(mktemp /tmp/seed_users.XXXXXX.sql)
trap 'rm -f "$SQL"' EXIT

for ((b = 0; b < BATCHES; b++)); do
  START=$((b * BATCH + 1))
  END=$(( (b + 1) * BATCH ))
  [ $END -gt $COUNT ] && END=$COUNT
  echo "INSERT INTO users(name, email) VALUES" >> "$SQL"
  for ((i = START; i <= END; i++)); do
    FI=$(( (i - 1) % NF ))
    LI=$(( ((i - 1) / NF) % NL ))
    FN=${FIRST[$FI]}
    LN=${LAST[$LI]}
    ROW="  ('${FN} ${LN}','${FN}.${LN}.${i}@example.com')"
    if [ $i -eq $END ]; then
      echo "${ROW};" >> "$SQL"
    else
      echo "${ROW}," >> "$SQL"
    fi
  done
done

mysql -h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" -p"$DB_PASS" "$DB_NAME" < "$SQL"

RESULT=$(mysql -h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" -p"$DB_PASS" "$DB_NAME" -Nse "SELECT COUNT(*) FROM users")
echo "✓ users table now holds ${RESULT} rows (target was ${COUNT})"
