DELETE FROM api_keys WHERE status='REVOKED';

INSERT INTO pricing_rules(model, size, quality, credits, is_active)
VALUES
  ('gpt-image-2', '1k', 'low', 4, true),
  ('gpt-image-2', '1k', 'medium', 8, true),
  ('gpt-image-2', '1k', 'high', 16, true),
  ('gpt-image-2', '1k', 'auto', 8, true),
  ('gpt-image-2', '2k', 'low', 8, true),
  ('gpt-image-2', '2k', 'medium', 16, true),
  ('gpt-image-2', '2k', 'high', 32, true),
  ('gpt-image-2', '2k', 'auto', 16, true),
  ('gpt-image-2', '4k', 'low', 16, true),
  ('gpt-image-2', '4k', 'medium', 32, true),
  ('gpt-image-2', '4k', 'high', 64, true),
  ('gpt-image-2', '4k', 'auto', 32, true)
ON CONFLICT (model, size, quality)
DO UPDATE SET credits=EXCLUDED.credits, is_active=true, updated_at=now();

UPDATE pricing_rules
SET is_active=false, updated_at=now()
WHERE model='gpt-image-2' AND size NOT IN ('1k', '2k', '4k');
