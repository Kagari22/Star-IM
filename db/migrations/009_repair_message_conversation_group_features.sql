-- Repairs databases where migration 008 was interrupted after its ALTER TABLE
-- statements completed. Each statement is safe to run when its table exists.

CREATE TABLE IF NOT EXISTS message_favorites (
  user_id BIGINT NOT NULL,
  message_id BIGINT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, message_id),
  CONSTRAINT fk_message_favorites_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_message_favorites_message FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS message_pins (
  user_id BIGINT NOT NULL,
  message_id BIGINT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, message_id),
  CONSTRAINT fk_message_pins_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_message_pins_message FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS user_message_hides (
  user_id BIGINT NOT NULL,
  message_id BIGINT NOT NULL,
  hidden_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, message_id),
  CONSTRAINT fk_user_message_hides_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_user_message_hides_message FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS conversation_reads (
  user_id BIGINT NOT NULL,
  peer_id BIGINT NOT NULL,
  last_read_message_id BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, peer_id),
  CONSTRAINT fk_conversation_reads_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_conversation_reads_peer FOREIGN KEY (peer_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS conversation_preferences (
  conversation_key VARCHAR(96) NOT NULL,
  user_id BIGINT NOT NULL,
  peer_id BIGINT NULL,
  group_id BIGINT NULL,
  is_pinned BOOLEAN NOT NULL DEFAULT FALSE,
  is_muted BOOLEAN NOT NULL DEFAULT FALSE,
  notifications_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  remark VARCHAR(64) NOT NULL DEFAULT '',
  category VARCHAR(64) NOT NULL DEFAULT '',
  hidden_at DATETIME NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (conversation_key),
  KEY idx_conversation_preferences_user (user_id),
  CONSTRAINT fk_conversation_preferences_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_conversation_preferences_peer FOREIGN KEY (peer_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_conversation_preferences_group FOREIGN KEY (group_id) REFERENCES chat_groups(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS group_kick_logs (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT NOT NULL,
  operator_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fk_group_kick_logs_group FOREIGN KEY (group_id) REFERENCES chat_groups(id) ON DELETE CASCADE,
  CONSTRAINT fk_group_kick_logs_operator FOREIGN KEY (operator_id) REFERENCES users(id),
  CONSTRAINT fk_group_kick_logs_user FOREIGN KEY (user_id) REFERENCES users(id),
  KEY idx_group_kick_logs_group_created (group_id, created_at)
);
