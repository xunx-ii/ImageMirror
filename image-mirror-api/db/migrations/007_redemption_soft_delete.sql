ALTER TABLE redemption_codes
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_redemption_codes_deleted_at ON redemption_codes(deleted_at);
