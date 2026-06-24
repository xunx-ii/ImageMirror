CREATE TABLE IF NOT EXISTS usage_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  user_email TEXT NOT NULL DEFAULT '',
  api_key_id UUID REFERENCES api_keys(id) ON DELETE SET NULL,
  api_key_name TEXT,
  api_key_prefix TEXT,
  image_generation_id UUID REFERENCES image_generations(id) ON DELETE SET NULL,
  source TEXT NOT NULL DEFAULT 'WEB',
  method TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT '',
  ip_address TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  prompt TEXT NOT NULL DEFAULT '',
  size TEXT NOT NULL DEFAULT '',
  quality TEXT NOT NULL DEFAULT '',
  reference_count INTEGER NOT NULL DEFAULT 0 CHECK (reference_count >= 0),
  credits_cost BIGINT NOT NULL DEFAULT 0 CHECK (credits_cost >= 0),
  status TEXT NOT NULL DEFAULT 'PENDING',
  success BOOLEAN NOT NULL DEFAULT false,
  status_code INTEGER,
  duration_ms BIGINT,
  error_message TEXT,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_usage_logs_created ON usage_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_logs_user_created ON usage_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_logs_success_created ON usage_logs(success, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_logs_image_generation_id
  ON usage_logs(image_generation_id)
  WHERE image_generation_id IS NOT NULL;

INSERT INTO system_config(key, value)
VALUES ('usage_log_retention_days', '90')
ON CONFLICT (key) DO NOTHING;
