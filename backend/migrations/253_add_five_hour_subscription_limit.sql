-- 每 5 小时订阅限额：与日/周/月限额并列，按订阅者独立累计。
ALTER TABLE groups ADD COLUMN IF NOT EXISTS five_hour_limit_usd DECIMAL(20,8) DEFAULT NULL;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS five_hour_window_start TIMESTAMPTZ;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS five_hour_usage_usd DECIMAL(20,10) NOT NULL DEFAULT 0;
