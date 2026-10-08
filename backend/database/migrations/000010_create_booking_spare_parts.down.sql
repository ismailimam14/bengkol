-- 000010_create_booking_spare_parts.down.sql

DROP TABLE IF EXISTS booking_spare_parts;
ALTER TABLE bookings DROP COLUMN IF EXISTS total_price;
