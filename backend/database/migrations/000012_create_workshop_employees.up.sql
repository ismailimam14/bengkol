-- 000012_create_workshop_employees.up.sql

CREATE TABLE IF NOT EXISTS workshop_employees (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workshop_id UUID NOT NULL REFERENCES workshops(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(100),
    phone VARCHAR(20) NOT NULL,
    role VARCHAR(50) NOT NULL CHECK (role IN ('MECHANIC', 'ADMIN_CASHIER', 'ADMIN_INVENTORY', 'ADMIN_BOTH', 'MANAGER', 'OWNER')),
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    specialization VARCHAR(100),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_workshop_employees_workshop_id ON workshop_employees(workshop_id);
CREATE INDEX IF NOT EXISTS idx_workshop_employees_user_id ON workshop_employees(user_id);
CREATE INDEX IF NOT EXISTS idx_workshop_employees_role ON workshop_employees(role);
CREATE INDEX IF NOT EXISTS idx_workshop_employees_status ON workshop_employees(status);
