-- 0001_init.down.sql

BEGIN;

DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS invoice_xml_files;
DROP TABLE IF EXISTS invoices;
DROP TABLE IF EXISTS accounts_receivable;
DROP TABLE IF EXISTS accounts_payable;
DROP TABLE IF EXISTS revenues;
DROP TABLE IF EXISTS expenses;
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS sale_items;
DROP TABLE IF EXISTS sales;
DROP TABLE IF EXISTS cash_sessions;
DROP TABLE IF EXISTS cash_registers;
DROP TABLE IF EXISTS inventory_movements;
DROP TABLE IF EXISTS inventory_balances;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS companies;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;

COMMIT;
