-- 000013_workshop_roles_permissions_separation.down.sql

DROP TRIGGER IF EXISTS trg_check_user_owner_phone ON users;
DROP TRIGGER IF EXISTS trg_check_employee_phone ON workshop_employees;
DROP FUNCTION IF EXISTS check_owner_employee_phone_separation();

ALTER TABLE workshop_employees DROP COLUMN IF EXISTS permissions;

ALTER TABLE workshop_employees DROP CONSTRAINT IF EXISTS workshop_employees_role_check;
ALTER TABLE workshop_employees ADD CONSTRAINT workshop_employees_role_check 
    CHECK (role IN ('MECHANIC', 'ADMIN_CASHIER', 'ADMIN_INVENTORY', 'ADMIN_BOTH', 'MANAGER', 'OWNER'));
