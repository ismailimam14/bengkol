-- 000007_make_users_email_optional.down.sql

DROP INDEX IF EXISTS idx_users_phone;
CREATE INDEX IF NOT EXISTS idx_users_phone ON users(phone);
ALTER TABLE users ALTER COLUMN email SET NOT NULL;
