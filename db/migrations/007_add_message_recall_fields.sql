ALTER TABLE messages
  ADD COLUMN recalled_at DATETIME NULL AFTER created_at,
  ADD COLUMN recalled_by BIGINT NULL AFTER recalled_at,
  ADD COLUMN recall_reason VARCHAR(255) NOT NULL DEFAULT '' AFTER recalled_by,
  ADD CONSTRAINT fk_messages_recalled_by
    FOREIGN KEY (recalled_by) REFERENCES users(id),
  ADD KEY idx_messages_recalled_at (recalled_at);
