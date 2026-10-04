-- 000008_add_photos_to_workshops.up.sql

ALTER TABLE workshops ADD COLUMN IF NOT EXISTS photos TEXT[] NOT NULL DEFAULT '{}';
