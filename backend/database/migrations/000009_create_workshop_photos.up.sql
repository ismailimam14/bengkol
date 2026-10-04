-- 000009_create_workshop_photos.up.sql

CREATE TABLE IF NOT EXISTS workshop_photos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workshop_id UUID NOT NULL REFERENCES workshops(id) ON DELETE CASCADE,
    data BYTEA NOT NULL,
    content_type VARCHAR(100) NOT NULL DEFAULT 'image/jpeg',
    filename VARCHAR(255) NOT NULL DEFAULT '',
    byte_size INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_workshop_photos_workshop_id ON workshop_photos(workshop_id);
