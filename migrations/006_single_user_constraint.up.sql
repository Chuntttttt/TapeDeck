-- Enforce single-user constraint
-- This application is designed for single-user household use
CREATE TRIGGER prevent_multiple_users
BEFORE INSERT ON users
WHEN (SELECT COUNT(*) FROM users) >= 1
BEGIN
    SELECT RAISE(FAIL, 'Only one user allowed. TapeDeck is designed for single-user operation.');
END;
