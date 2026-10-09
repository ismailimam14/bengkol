-- 000011_create_vehicles.down.sql

DROP INDEX IF EXISTS idx_bookings_vehicle_id;
ALTER TABLE bookings DROP COLUMN IF EXISTS vehicle_id;
DROP TABLE IF EXISTS vehicles;
