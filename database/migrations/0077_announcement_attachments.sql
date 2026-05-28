-- 0077_announcement_attachments.sql

ALTER TABLE announcements
    ADD COLUMN IF NOT EXISTS attachment_filename TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attachment_content_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attachment_size_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS attachment_sha256 TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attachment_data BYTEA;
