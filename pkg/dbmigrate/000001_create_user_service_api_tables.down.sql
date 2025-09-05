-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS api_key_statuses CASCADE;
DROP TABLE IF EXISTS services CASCADE;
DROP TABLE IF EXISTS users CASCADE;
