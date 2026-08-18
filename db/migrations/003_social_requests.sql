CREATE TABLE IF NOT EXISTS friendships (
  user_id BIGINT NOT NULL,
  friend_id BIGINT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, friend_id),
  CONSTRAINT fk_friendships_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_friendships_friend FOREIGN KEY (friend_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS friend_requests (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  from_user_id BIGINT NOT NULL,
  to_user_id BIGINT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  responded_at DATETIME NULL,
  CONSTRAINT fk_friend_requests_from FOREIGN KEY (from_user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_friend_requests_to FOREIGN KEY (to_user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT chk_friend_requests_status CHECK (status IN ('pending', 'accepted', 'rejected')),
  KEY idx_friend_requests_to_status (to_user_id, status),
  KEY idx_friend_requests_pair_status (from_user_id, to_user_id, status)
);

CREATE TABLE IF NOT EXISTS group_join_requests (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  responded_at DATETIME NULL,
  CONSTRAINT fk_group_join_requests_group FOREIGN KEY (group_id) REFERENCES chat_groups(id) ON DELETE CASCADE,
  CONSTRAINT fk_group_join_requests_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT chk_group_join_requests_status CHECK (status IN ('pending', 'accepted', 'rejected')),
  KEY idx_group_join_requests_group_status (group_id, status),
  KEY idx_group_join_requests_user_group (user_id, group_id, status)
);
