-- A message can be edited more than once. Outbox records are immutable and
-- therefore need one row for every edit, not one row per event type/message.
ALTER TABLE outbox_events DROP INDEX uq_outbox_event_message;
ALTER TABLE outbox_events ADD KEY idx_outbox_event_message (event_type, message_id);
