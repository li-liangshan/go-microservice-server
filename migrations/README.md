# migrations

SQL files are versioned in numeric order (`0001_*.sql`, `0002_*.sql`, …).
Each file should be idempotent so it can be re-applied safely without a
migration tracker.

## Apply manually

```bash
# One-off, against the configured DB:
mysql -h127.0.0.1 -P3306 -uroot -padmin < migrations/0001_init.sql

# Loop through every file in order:
for f in migrations/*.sql; do
  mysql -h127.0.0.1 -P3306 -uroot -padmin < "$f"
done
```

## Catalog

| File | Description |
|---|---|
| `0001_init.sql` | Creates the `microservice` database and the `users` table. |

## Moving to a real migration tool

For anything beyond a one-table template, switch to a versioned migration
runner. Drop-in options that work with this layout:

- [golang-migrate](https://github.com/golang-migrate/migrate) — pure Go, CLI + library
- [goose](https://github.com/pressly/goose) — Go + SQL, embeds into the binary
- [atlas](https://atlasgo.io) — declarative schema diffing

Both expect files numbered exactly like this (`NNN_*.sql` with `.up.sql` /
`.down.sql` pairs). Renaming `0001_init.sql` → `0001_init.up.sql` and adding a
matching `.down.sql` is all that's needed to adopt them.
