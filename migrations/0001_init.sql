-- ───────────────────────────────────────────────────────────────────────────
-- 0001_init.sql — initial schema for microservice-server
-- Creates the database and the `users` table.
-- ───────────────────────────────────────────────────────────────────────────
-- Apply:   mysql -h<host> -P<port> -u<user> -p<pwd> < migrations/0001_init.sql
-- Re-runnable (CREATE … IF NOT EXISTS).
-- ───────────────────────────────────────────────────────────────────────────

CREATE DATABASE IF NOT EXISTS `microservice`
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;

USE `microservice`;

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
