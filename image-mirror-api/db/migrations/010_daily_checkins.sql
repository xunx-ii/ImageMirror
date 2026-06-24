CREATE TABLE IF NOT EXISTS daily_checkins (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  checkin_date DATE NOT NULL,
  credits BIGINT NOT NULL CHECK (credits > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, checkin_date)
);

CREATE INDEX IF NOT EXISTS idx_daily_checkins_user_created ON daily_checkins(user_id, created_at DESC);

INSERT INTO system_config(key, value)
VALUES
  ('daily_checkin_enabled', 'false'),
  ('daily_checkin_credits', '5')
ON CONFLICT (key) DO NOTHING;
