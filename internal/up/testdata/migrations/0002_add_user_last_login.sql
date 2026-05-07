-- 0002_add_user_last_login.sql
-- Add last_login column to users table

ALTER TABLE users ADD COLUMN last_login TIMESTAMP;

-- Index for frequently querying by last_login
CREATE INDEX idx_users_last_login ON users(last_login);
