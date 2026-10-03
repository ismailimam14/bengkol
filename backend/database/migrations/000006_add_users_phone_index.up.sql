-- 000006_add_users_phone_index.up.sql

CREATE INDEX IF NOT EXISTS idx_users_phone ON users(phone);
