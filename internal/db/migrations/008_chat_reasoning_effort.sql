-- +goose Up
ALTER TABLE chat_conversations ADD COLUMN reasoning_effort TEXT;

-- +goose Down
SELECT 1;
