-- 000007_make_users_email_optional.up.sql

ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
DROP INDEX IF EXISTS idx_users_phone;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone ON users(phone);
