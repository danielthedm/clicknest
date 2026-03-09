CREATE TABLE project_members (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member' CHECK(role IN ('owner','member')),
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, project_id)
);

ALTER TABLE user_sessions ADD COLUMN project_id TEXT REFERENCES projects(id);

-- Backfill: link existing users to projects as owner
INSERT OR IGNORE INTO project_members (user_id, project_id, role)
  SELECT u.id, p.id, 'owner' FROM users u CROSS JOIN projects p;

-- Backfill: set project_id on existing sessions
UPDATE user_sessions SET project_id = (
  SELECT pm.project_id FROM project_members pm
  WHERE pm.user_id = user_sessions.user_id
  ORDER BY pm.created_at LIMIT 1
) WHERE project_id IS NULL;
