CREATE TABLE IF NOT EXISTS admin_notes (
  note_id SERIAL PRIMARY KEY,
  note_date DATE NOT NULL,
  title VARCHAR(200) NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_notes_note_date ON admin_notes(note_date DESC, updated_at DESC);
