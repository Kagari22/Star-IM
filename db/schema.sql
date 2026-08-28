CREATE DATABASE IF NOT EXISTS im_chat
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_unicode_ci;

USE im_chat;

CREATE TABLE IF NOT EXISTS users (
  id BIGINT PRIMARY KEY,
  username VARCHAR(64) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  nickname VARCHAR(64) NOT NULL,
  avatar_key VARCHAR(255) NOT NULL DEFAULT '',
  balance BIGINT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS chat_groups (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(64) NOT NULL,
  owner_id BIGINT NOT NULL,
  avatar_key VARCHAR(255) NOT NULL DEFAULT '',
  avatar_url VARCHAR(512) NOT NULL DEFAULT '',
  announcement VARCHAR(500) NOT NULL DEFAULT '',
  all_muted BOOLEAN NOT NULL DEFAULT FALSE,
  invite_code VARCHAR(32) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_chat_groups_owner FOREIGN KEY (owner_id) REFERENCES users(id),
  KEY idx_chat_groups_owner_id (owner_id)
);

CREATE TABLE IF NOT EXISTS group_members (
  group_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  role VARCHAR(16) NOT NULL DEFAULT 'member',
  joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_read_message_id BIGINT NOT NULL DEFAULT 0,
  muted_until DATETIME NULL,
  PRIMARY KEY (group_id, user_id),
  CONSTRAINT fk_group_members_group FOREIGN KEY (group_id) REFERENCES chat_groups(id) ON DELETE CASCADE,
  CONSTRAINT fk_group_members_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT chk_group_members_role CHECK (role IN ('owner', 'admin', 'member')),
  KEY idx_group_members_user_group (user_id, group_id)
);

CREATE TABLE IF NOT EXISTS messages (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  from_user_id BIGINT NOT NULL,
  to_user_id BIGINT NULL,
  group_id BIGINT NULL,
  content_type VARCHAR(32) NOT NULL DEFAULT 'text',
  content TEXT NOT NULL,
  object_key VARCHAR(255) NOT NULL DEFAULT '',
  object_url VARCHAR(512) NOT NULL DEFAULT '',
  file_name VARCHAR(255) NOT NULL DEFAULT '',
  file_size BIGINT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  recalled_at DATETIME NULL,
  recalled_by BIGINT NULL,
  recall_reason VARCHAR(255) NOT NULL DEFAULT '',
  reply_to_id BIGINT NULL,
  edited_at DATETIME NULL,
  CONSTRAINT fk_messages_from_user FOREIGN KEY (from_user_id) REFERENCES users(id),
  CONSTRAINT fk_messages_to_user FOREIGN KEY (to_user_id) REFERENCES users(id),
  CONSTRAINT fk_messages_group FOREIGN KEY (group_id) REFERENCES chat_groups(id),
  CONSTRAINT fk_messages_recalled_by FOREIGN KEY (recalled_by) REFERENCES users(id),
  CONSTRAINT fk_messages_reply_to FOREIGN KEY (reply_to_id) REFERENCES messages(id) ON DELETE SET NULL,
  CONSTRAINT chk_messages_target CHECK (
    (to_user_id IS NOT NULL AND group_id IS NULL)
    OR
    (to_user_id IS NULL AND group_id IS NOT NULL)
  ),
  KEY idx_pair_id (from_user_id, to_user_id, id),
  KEY idx_to_user_id_id (to_user_id, id),
  KEY idx_messages_group_id_id (group_id, id),
  KEY idx_messages_recalled_at (recalled_at)
  ,KEY idx_messages_reply_to_id (reply_to_id)
);

CREATE TABLE IF NOT EXISTS user_message_hides (
  user_id BIGINT NOT NULL,
  message_id BIGINT NOT NULL,
  hidden_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, message_id),
  CONSTRAINT fk_user_message_hides_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_user_message_hides_message FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
);

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
  KEY idx_outbox_event_message (event_type, message_id),
  KEY idx_outbox_pending (published_at, available_at, locked_until, id),
  CONSTRAINT fk_outbox_message FOREIGN KEY (message_id) REFERENCES messages(id)
);

CREATE TABLE IF NOT EXISTS friendships (
  user_id BIGINT NOT NULL,
  friend_id BIGINT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, friend_id),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (friend_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS friend_requests (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  from_user_id BIGINT NOT NULL,
  to_user_id BIGINT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  responded_at DATETIME NULL,
  FOREIGN KEY (from_user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (to_user_id) REFERENCES users(id) ON DELETE CASCADE,
  KEY idx_friend_requests_to_status (to_user_id, status)
);

CREATE TABLE IF NOT EXISTS group_join_requests (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  responded_at DATETIME NULL,
  FOREIGN KEY (group_id) REFERENCES chat_groups(id) ON DELETE CASCADE,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  KEY idx_group_join_requests_group_status (group_id, status)
);

CREATE TABLE IF NOT EXISTS redeem_codes (
  code VARCHAR(64) NOT NULL,
  amount BIGINT NOT NULL,
  used_by BIGINT NULL,
  used_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (code),
  CONSTRAINT fk_redeem_codes_used_by FOREIGN KEY (used_by) REFERENCES users(id)
);

INSERT IGNORE INTO redeem_codes (code, amount) VALUES ('114514', 10000);

CREATE TABLE IF NOT EXISTS red_packets (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  sender_id BIGINT NOT NULL,
  group_id BIGINT NULL,
  receiver_id BIGINT NULL,
  total_amount BIGINT NOT NULL,
  total_count INT NOT NULL,
  remaining_amount BIGINT NOT NULL,
  remaining_count INT NOT NULL,
  lucky TINYINT(1) NOT NULL DEFAULT 1,
  greeting VARCHAR(100) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fk_red_packets_sender FOREIGN KEY (sender_id) REFERENCES users(id),
  CONSTRAINT fk_red_packets_group FOREIGN KEY (group_id) REFERENCES chat_groups(id) ON DELETE CASCADE,
  CONSTRAINT fk_red_packets_receiver FOREIGN KEY (receiver_id) REFERENCES users(id),
  KEY idx_red_packets_group (group_id),
  KEY idx_red_packets_sender (sender_id)
);

CREATE TABLE IF NOT EXISTS red_packet_receipts (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  packet_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  amount BIGINT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_red_packet_receipts_packet_user (packet_id, user_id),
  CONSTRAINT fk_red_packet_receipts_packet FOREIGN KEY (packet_id) REFERENCES red_packets(id) ON DELETE CASCADE,
  CONSTRAINT fk_red_packet_receipts_user FOREIGN KEY (user_id) REFERENCES users(id)
);
