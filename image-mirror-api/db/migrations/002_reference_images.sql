ALTER TABLE image_generations
ADD COLUMN IF NOT EXISTS reference_keys JSONB NOT NULL DEFAULT '[]'::jsonb;
