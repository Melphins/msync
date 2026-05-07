-- 0005_add_user_preferences.sql
-- Store user preferences as JSON

ALTER TABLE users ADD COLUMN preferences JSONB DEFAULT '{}';

-- Example query: SELECT preferences->'theme' FROM users WHERE id = 1;
