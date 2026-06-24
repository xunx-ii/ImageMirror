INSERT INTO site_content(key, title, body, is_active)
VALUES
  ('terms', '服务条款', '# 服务条款\n\n请在后台编辑这里的服务条款内容。', true),
  ('privacy', '隐私政策', '# 隐私政策\n\n请在后台编辑这里的隐私政策内容。', true)
ON CONFLICT (key) DO NOTHING;
