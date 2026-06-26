ALTER TABLE openai_endpoints
  ADD COLUMN IF NOT EXISTS supports_streaming BOOLEAN NOT NULL DEFAULT true;
