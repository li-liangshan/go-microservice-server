#!/usr/bin/env bash
# Apply every SQL migration in numeric order.
#
# Connection settings use the same DATABASE_* environment variables as the
# service. Passwords are passed through MYSQL_PWD so they do not appear in the
# process list.

set -euo pipefail

DB_HOST="${DATABASE_HOST:-127.0.0.1}"
DB_PORT="${DATABASE_PORT:-3306}"
DB_USER="${DATABASE_USER:-root}"
DB_PASS="${DATABASE_PASSWORD:-}"

if ! command -v mysql >/dev/null 2>&1; then
	echo "mysql client is required to apply migrations" >&2
	exit 1
fi

shopt -s nullglob
files=(migrations/*.sql)
if [ ${#files[@]} -eq 0 ]; then
	echo "no migrations found" >&2
	exit 1
fi

for file in "${files[@]}"; do
	echo "applying ${file}"
	MYSQL_PWD="$DB_PASS" mysql --protocol=tcp \
		-h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" <"$file"
done

echo "migrations applied"
