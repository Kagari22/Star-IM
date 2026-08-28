-- 红包功能：用户余额、兑换码、红包与领取记录
-- 金额一律以「分」为单位存储（BIGINT），避免浮点误差。

ALTER TABLE users ADD COLUMN balance BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS redeem_codes (
  code VARCHAR(64) NOT NULL,
  amount BIGINT NOT NULL,
  used_by BIGINT NULL,
  used_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (code),
  CONSTRAINT fk_redeem_codes_used_by FOREIGN KEY (used_by) REFERENCES users(id)
);

-- 预置兑换码 114514 -> 100 元（10000 分）
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
