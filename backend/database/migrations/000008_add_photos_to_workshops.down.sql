-- 000008_add_photos_to_workshops.down.sql

ALTER TABLE workshops DROP COLUMN IF EXISTS photos;
