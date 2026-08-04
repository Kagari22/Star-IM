CREATE DATABASE IF NOT EXISTS im_chat
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_unicode_ci;

USE im_chat;

CREATE TABLE IF NOT EXISTS users (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  username VARCHAR(64) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  nickname VARCHAR(64) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  from_user_id BIGINT NOT NULL,
  to_user_id BIGINT NOT NULL,
  content_type VARCHAR(32) NOT NULL DEFAULT 'text',
  content TEXT NOT NULL,
  object_key VARCHAR(255) NOT NULL DEFAULT '',
  object_url VARCHAR(512) NOT NULL DEFAULT '',
  file_name VARCHAR(255) NOT NULL DEFAULT '',
  file_size BIGINT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fk_messages_from_user FOREIGN KEY (from_user_id) REFERENCES users(id),
  CONSTRAINT fk_messages_to_user FOREIGN KEY (to_user_id) REFERENCES users(id),
  KEY idx_pair_id (from_user_id, to_user_id, id),
  KEY idx_to_user_id_id (to_user_id, id)
);

CREATE TABLE IF NOT EXISTS outbox_events (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  event_type VARCHAR(64) NOT NULL,
  message_id BIGINT NOT NULL,
  payload JSON NOT NULL,
  attempts INT NOT NULL DEFAULT 0,
  available_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  locked_until DATETIME NULL,
  published_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_outbox_event_message (event_type, message_id),
  KEY idx_outbox_pending (published_at, available_at, locked_until, id),
  CONSTRAINT fk_outbox_message FOREIGN KEY (message_id) REFERENCES messages(id)
);
