-- 000010_create_booking_spare_parts.up.sql

ALTER TABLE bookings ADD COLUMN IF NOT EXISTS total_price NUMERIC(12, 2) NOT NULL DEFAULT 0.00;

CREATE TABLE IF NOT EXISTS booking_spare_parts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    spare_part_id UUID NOT NULL REFERENCES spare_parts(id) ON DELETE RESTRICT,
    quantity INT NOT NULL DEFAULT 1 CHECK (quantity > 0),
    price_per_unit NUMERIC(12, 2) NOT NULL CHECK (price_per_unit >= 0),
    subtotal NUMERIC(12, 2) NOT NULL CHECK (subtotal >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_booking_spare_parts_booking_id ON booking_spare_parts(booking_id);
CREATE INDEX IF NOT EXISTS idx_booking_spare_parts_spare_part_id ON booking_spare_parts(spare_part_id);
