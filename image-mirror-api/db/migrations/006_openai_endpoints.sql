CREATE TABLE IF NOT EXISTS openai_endpoints (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  api_key TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  schedulable BOOLEAN NOT NULL DEFAULT true,
  priority INTEGER NOT NULL DEFAULT 100,
  failure_count INTEGER NOT NULL DEFAULT 0 CHECK (failure_count >= 0),
  circuit_open_until TIMESTAMPTZ,
  last_error TEXT,
  last_used_at TIMESTAMPTZ,
  last_success_at TIMESTAMPTZ,
  last_failure_at TIMESTAMPTZ,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_openai_endpoints_schedule
  ON openai_endpoints(enabled, schedulable, priority, last_used_at);

INSERT INTO openai_endpoints(name, base_url, api_key, enabled, schedulable, priority, updated_by)
SELECT
  '默认节点',
  COALESCE(NULLIF(trim(base.value), ''), 'https://api.openai.com'),
  trim(key.value),
  true,
  true,
  100,
  key.updated_by
FROM system_config key
LEFT JOIN system_config base ON base.key = 'openai_base_url'
WHERE key.key = 'openai_api_key'
  AND trim(key.value) <> ''
  AND NOT EXISTS (SELECT 1 FROM openai_endpoints);
