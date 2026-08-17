-- Reverse migration for 0001_init.sql. Drop in reverse dependency order.
DROP TABLE IF EXISTS config_versions;
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS batch_jobs;
DROP TABLE IF EXISTS pricing_decisions;
DROP TABLE IF EXISTS promotions;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
