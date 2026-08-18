CREATE TABLE IF NOT EXISTS chat_groups (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(64) NOT NULL,
  owner_id BIGINT NOT NULL,
  avatar_url VARCHAR(512) NOT NULL DEFAULT '',
  announcement VARCHAR(500) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  CONSTRAINT fk_chat_groups_owner
    FOREIGN KEY (owner_id) REFERENCES users(id),

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

  CONSTRAINT fk_group_members_group
    FOREIGN KEY (group_id) REFERENCES chat_groups(id)
    ON DELETE CASCADE,

  CONSTRAINT fk_group_members_user
    FOREIGN KEY (user_id) REFERENCES users(id)
    ON DELETE CASCADE,

  CONSTRAINT chk_group_members_role
    CHECK (role IN ('owner', 'admin', 'member')),

  KEY idx_group_members_user_group (user_id, group_id)
);

ALTER TABLE messages
  MODIFY COLUMN to_user_id BIGINT NULL,
  ADD COLUMN group_id BIGINT NULL AFTER to_user_id,
  ADD KEY idx_messages_group_id_id (group_id, id),
  ADD CONSTRAINT fk_messages_group
    FOREIGN KEY (group_id) REFERENCES chat_groups(id),
  ADD CONSTRAINT chk_messages_target
    CHECK (
      (to_user_id IS NOT NULL AND group_id IS NULL)
      OR
      (to_user_id IS NULL AND group_id IS NOT NULL)
    );

