#!/usr/bin/env bash
# Apply every SQL migration in numeric order.
#
# Connection settings use the same DATABASE_* environment variables as the
# service. Passwords are passed through MYSQL_PWD so they do not appear in
# the process list.
#
# The target database is taken from DATABASE_NAME and created when missing, so
# the schema follows the configuration instead of a hard-coded name.
#
# Applied files are recorded in the `schema_migrations` table and are not
# re-applied, which makes the script safe to run repeatedly and lets it
# report which version the database is at.

set -euo pipefail

DB_HOST="${DATABASE_HOST:-127.0.0.1}"
DB_PORT="${DATABASE_PORT:-3306}"
DB_USER="${DATABASE_USER:-root}"
DB_PASS="${DATABASE_PASSWORD:-}"
DB_NAME="${DATABASE_NAME:-microservice}"

if ! command -v mysql >/dev/null 2>&1; then
	echo "mysql client is required to apply migrations" >&2
	exit 1
fi

# Run a statement against the server without selecting a database.
mysql_root() {
	MYSQL_PWD="$DB_PASS" mysql --protocol=tcp \
		-h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" "$@"
}

# Run a statement against the target database.
mysql_db() {
	MYSQL_PWD="$DB_PASS" mysql --protocol=tcp \
		-h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" --database="$DB_NAME" "$@"
}

shopt -s nullglob
files=(migrations/*.sql)
if [ ${#files[@]} -eq 0 ]; then
	echo "no migrations found" >&2
	exit 1
fi

echo "ensuring database '$DB_NAME' exists"
mysql_root -e "CREATE DATABASE IF NOT EXISTS \`$DB_NAME\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

mysql_db -e "
CREATE TABLE IF NOT EXISTS \`schema_migrations\` (
  \`version\`     VARCHAR(255) NOT NULL,
  \`applied_at\`  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (\`version\`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

applied_count=0
for file in "${files[@]}"; do
	version="$(basename "$file" .sql)"

	already=$(mysql_db -Nse "SELECT COUNT(*) FROM \`schema_migrations\` WHERE \`version\` = '$version';")
	if [ "$already" != "0" ]; then
		echo "skipping $version (already applied)"
		continue
	fi

	echo "applying $version"
	mysql_db <"$file"
	mysql_db -e "INSERT INTO \`schema_migrations\` (\`version\`) VALUES ('$version');"
	applied_count=$((applied_count + 1))
done

current=$(mysql_db -Nse "SELECT \`version\` FROM \`schema_migrations\` ORDER BY \`version\` DESC LIMIT 1;")
echo "migrations applied ($applied_count new); database '$DB_NAME' is at version '${current:-none}'"
