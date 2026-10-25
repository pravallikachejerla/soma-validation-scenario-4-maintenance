-- Rollback for migration 0002: drops the four override tables.
-- No existing data is affected.

DROP TABLE IF EXISTS override_audit;
DROP TABLE IF EXISTS override_approvals;
DROP TABLE IF EXISTS override_requests;
DROP TABLE IF EXISTS override_settings;
