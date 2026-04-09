-- +goose Up
ALTER TABLE chat_conversations ADD COLUMN context_summary TEXT;
ALTER TABLE chat_conversations ADD COLUMN compacted_message_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_conversations ADD COLUMN compaction_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_conversations ADD COLUMN compacted_at DATETIME;
ALTER TABLE chat_conversations ADD COLUMN context_window INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_conversations ADD COLUMN context_token_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_conversations ADD COLUMN last_prompt_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_conversations ADD COLUMN last_completion_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_conversations ADD COLUMN last_total_tokens INTEGER NOT NULL DEFAULT 0;

ALTER TABLE chat_messages ADD COLUMN context_messages TEXT;

-- +goose Down
SELECT 1;
