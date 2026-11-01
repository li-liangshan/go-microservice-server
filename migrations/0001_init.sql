-- ───────────────────────────────────────────────────────────────────────────
-- 0001_init.sql — initial schema for microservice-server
-- Creates the `users` table.
-- ───────────────────────────────────────────────────────────────────────────
-- Apply:   mysql -h<host> -P<port> -u<user> -p<pwd> --database=<db> < migrations/0001_init.sql
--
-- The target database is deliberately NOT named here: the database name is a
-- deployment setting (DATABASE_NAME) and is selected by the caller — the
-- migrate script passes --database, and the Docker entrypoint passes
-- --database="$MYSQL_DATABASE". Hard-coding it here would tie the schema to
-- one deployment name.
--
-- Re-runnable (CREATE … IF NOT EXISTS).
-- ───────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS `users` (
  `id`         BIGINT       NOT NULL AUTO_INCREMENT,
  `name`       VARCHAR(128) NOT NULL DEFAULT '',
  `email`      VARCHAR(255) NOT NULL DEFAULT '',
  `created_at` DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_users_email` (`email`)
) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci;
