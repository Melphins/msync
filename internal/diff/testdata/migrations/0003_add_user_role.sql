-- 0003_add_user_role.sql
-- Add role column with enum-like constraint

ALTER TABLE users ADD COLUMN role VARCHAR(50) DEFAULT 'user';

CREATE TABLE user_roles (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    description TEXT
);

INSERT INTO user_roles (name, description) VALUES
    ('admin', 'Administrator'),
    ('user', 'Regular user'),
    ('moderator', 'Content moderator');
