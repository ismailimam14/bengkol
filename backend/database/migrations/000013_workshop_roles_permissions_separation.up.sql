-- 000013_workshop_roles_permissions_separation.up.sql

-- 1. Update role check constraint on workshop_employees to include ADMIN and CUSTOMER
ALTER TABLE workshop_employees DROP CONSTRAINT IF EXISTS workshop_employees_role_check;
ALTER TABLE workshop_employees ADD CONSTRAINT workshop_employees_role_check 
    CHECK (role IN ('ADMIN', 'CUSTOMER', 'MECHANIC', 'MANAGER', 'OWNER', 'ADMIN_CASHIER', 'ADMIN_INVENTORY', 'ADMIN_BOTH'));

-- 2. Add permissions JSONB column to workshop_employees for configurable permissions
ALTER TABLE workshop_employees ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT NULL;

-- 3. Trigger & Function to prevent phone collisions between OWNER and workshop_employees
CREATE OR REPLACE FUNCTION check_owner_employee_phone_separation()
RETURNS TRIGGER AS $$
DECLARE
    v_norm_phone VARCHAR(50);
BEGIN
    -- Canonical phone normalization: strip non-digits except '+'
    v_norm_phone := regexp_replace(NEW.phone, '[^0-9+]', '', 'g');
    IF v_norm_phone LIKE '+62%' THEN
        v_norm_phone := '0' || substring(v_norm_phone FROM 4);
    ELSIF v_norm_phone LIKE '62%' AND length(v_norm_phone) > 9 THEN
        v_norm_phone := '0' || substring(v_norm_phone FROM 3);
    END IF;

    IF TG_TABLE_NAME = 'workshop_employees' THEN
        -- Reject if phone is an OWNER in users or is the owner of any workshop
        IF EXISTS (
            SELECT 1 FROM users u 
            WHERE (u.role = 'OWNER' OR EXISTS (SELECT 1 FROM workshops w WHERE w.owner_id = u.id))
              AND (
                  u.phone = NEW.phone 
                  OR regexp_replace(u.phone, '[^0-9+]', '', 'g') = v_norm_phone
                  OR (
                      CASE 
                          WHEN regexp_replace(u.phone, '[^0-9+]', '', 'g') LIKE '+62%' THEN '0' || substring(regexp_replace(u.phone, '[^0-9+]', '', 'g') FROM 4)
                          WHEN regexp_replace(u.phone, '[^0-9+]', '', 'g') LIKE '62%' AND length(regexp_replace(u.phone, '[^0-9+]', '', 'g')) > 9 THEN '0' || substring(regexp_replace(u.phone, '[^0-9+]', '', 'g') FROM 3)
                          ELSE regexp_replace(u.phone, '[^0-9+]', '', 'g')
                      END
                  ) = v_norm_phone
              )
        ) THEN
            RAISE EXCEPTION 'Phone number % is registered to a workshop OWNER and cannot be used for an employee account', NEW.phone;
        END IF;
    ELSIF TG_TABLE_NAME = 'users' THEN
        IF NEW.role = 'OWNER' THEN
            -- Reject if phone is registered in workshop_employees
            IF EXISTS (
                SELECT 1 FROM workshop_employees we 
                WHERE we.phone = NEW.phone 
                   OR regexp_replace(we.phone, '[^0-9+]', '', 'g') = v_norm_phone
                   OR (
                       CASE 
                           WHEN regexp_replace(we.phone, '[^0-9+]', '', 'g') LIKE '+62%' THEN '0' || substring(regexp_replace(we.phone, '[^0-9+]', '', 'g') FROM 4)
                           WHEN regexp_replace(we.phone, '[^0-9+]', '', 'g') LIKE '62%' AND length(regexp_replace(we.phone, '[^0-9+]', '', 'g')) > 9 THEN '0' || substring(regexp_replace(we.phone, '[^0-9+]', '', 'g') FROM 3)
                           ELSE regexp_replace(we.phone, '[^0-9+]', '', 'g')
                       END
                   ) = v_norm_phone
            ) THEN
                RAISE EXCEPTION 'Phone number % is registered to a workshop employee and cannot be used for an OWNER account', NEW.phone;
            END IF;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_check_employee_phone ON workshop_employees;
CREATE TRIGGER trg_check_employee_phone
    BEFORE INSERT OR UPDATE OF phone ON workshop_employees
    FOR EACH ROW EXECUTE FUNCTION check_owner_employee_phone_separation();

DROP TRIGGER IF EXISTS trg_check_user_owner_phone ON users;
CREATE TRIGGER trg_check_user_owner_phone
    BEFORE INSERT OR UPDATE OF phone, role ON users
    FOR EACH ROW EXECUTE FUNCTION check_owner_employee_phone_separation();
