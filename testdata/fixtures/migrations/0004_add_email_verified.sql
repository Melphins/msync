-- 0004_add_email_verified.sql
-- Track email verification status

ALTER TABLE users ADD COLUMN email_verified BOOLEAN DEFAULT false;
ALTER TABLE users ADD COLUMN verification_token VARCHAR(100);

CREATE INDEX idx_users_email_verified ON users(email_verified);
