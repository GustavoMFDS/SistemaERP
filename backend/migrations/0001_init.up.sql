-- 0001_init.up.sql

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Auth / RBAC
CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email citext NOT NULL UNIQUE,
  name text NOT NULL,
  password_hash text NOT NULL,
  active boolean NOT NULL DEFAULT true,
  last_login_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS roles (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  description text NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS permissions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code text NOT NULL UNIQUE,
  description text NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_roles (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role_id)
);

CREATE TABLE IF NOT EXISTS role_permissions (
  role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (role_id, permission_id)
);

-- Company (emitente)
CREATE TABLE IF NOT EXISTS companies (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  legal_name text NOT NULL,
  trade_name text NULL,
  cnpj text NOT NULL,
  ie text NULL,
  crt text NULL,

  address_street text NULL,
  address_number text NULL,
  address_complement text NULL,
  address_neighborhood text NULL,
  address_city text NULL,
  address_state text NULL,
  address_zip text NULL,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT companies_cnpj_len CHECK (char_length(cnpj) BETWEEN 11 AND 14)
);

-- Customers
CREATE TABLE IF NOT EXISTS customers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  document text NULL,
  email citext NULL,
  phone text NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS customers_document_unique ON customers(document) WHERE document IS NOT NULL;

-- Catalog
CREATE TABLE IF NOT EXISTS categories (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS products (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  category_id uuid NULL REFERENCES categories(id) ON DELETE SET NULL,
  sku text NOT NULL UNIQUE,
  barcode text NULL,
  name text NOT NULL,
  description text NULL,
  unit text NOT NULL,
  cost_price numeric(12,2) NOT NULL DEFAULT 0,
  price_cash numeric(12,2) NOT NULL,
  promo_price numeric(12,2) NULL,
  min_stock numeric(14,3) NOT NULL DEFAULT 0,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS products_barcode_unique ON products(barcode) WHERE barcode IS NOT NULL;
CREATE INDEX IF NOT EXISTS products_name_trgm_idx ON products USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS products_sku_trgm_idx ON products USING gin (sku gin_trgm_ops);
CREATE INDEX IF NOT EXISTS products_barcode_idx ON products(barcode);

-- Inventory
CREATE TABLE IF NOT EXISTS inventory_balances (
  product_id uuid PRIMARY KEY REFERENCES products(id) ON DELETE CASCADE,
  qty_on_hand numeric(14,3) NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS inventory_movements (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
  movement_type text NOT NULL, -- purchase, sale, adjustment, loss, damage, return
  delta numeric(14,3) NOT NULL,
  qty_before numeric(14,3) NOT NULL,
  qty_after numeric(14,3) NOT NULL,
  reason text NULL,
  reference_type text NULL,
  reference_id uuid NULL,
  actor_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT inv_movement_delta_nonzero CHECK (delta <> 0)
);
CREATE INDEX IF NOT EXISTS inventory_movements_product_created_idx ON inventory_movements(product_id, created_at DESC);

-- Cash
CREATE TABLE IF NOT EXISTS cash_registers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cash_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  cash_register_id uuid NOT NULL REFERENCES cash_registers(id) ON DELETE RESTRICT,
  opened_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  closed_by_user_id uuid NULL REFERENCES users(id) ON DELETE RESTRICT,
  opened_at timestamptz NOT NULL DEFAULT now(),
  closed_at timestamptz NULL,
  opening_amount numeric(12,2) NOT NULL DEFAULT 0,
  closing_amount numeric(12,2) NULL,
  status text NOT NULL DEFAULT 'open',
  notes text NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT cash_sessions_status_check CHECK (status IN ('open','closed'))
);
CREATE INDEX IF NOT EXISTS cash_sessions_status_idx ON cash_sessions(status, opened_at DESC);

-- Sales / PDV
CREATE TABLE IF NOT EXISTS sales (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  cash_session_id uuid NOT NULL REFERENCES cash_sessions(id) ON DELETE RESTRICT,
  customer_id uuid NULL REFERENCES customers(id) ON DELETE SET NULL,
  status text NOT NULL DEFAULT 'finalized',
  subtotal numeric(12,2) NOT NULL,
  discount_value numeric(12,2) NOT NULL DEFAULT 0,
  total numeric(12,2) NOT NULL,
  profit_estimated numeric(12,2) NOT NULL DEFAULT 0,
  created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  finalized_at timestamptz NOT NULL DEFAULT now(),
  cancelled_at timestamptz NULL,
  cancel_reason text NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sales_status_check CHECK (status IN ('open','finalized','cancelled')),
  CONSTRAINT sales_total_nonnegative CHECK (total >= 0)
);
CREATE INDEX IF NOT EXISTS sales_status_created_idx ON sales(status, created_at DESC);

CREATE TABLE IF NOT EXISTS sale_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sale_id uuid NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
  qty numeric(14,3) NOT NULL,
  unit_price numeric(12,2) NOT NULL,
  discount_value numeric(12,2) NOT NULL DEFAULT 0,
  subtotal numeric(12,2) NOT NULL,
  cost_unit numeric(12,2) NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sale_items_qty_positive CHECK (qty > 0),
  CONSTRAINT sale_items_subtotal_nonnegative CHECK (subtotal >= 0)
);
CREATE INDEX IF NOT EXISTS sale_items_sale_idx ON sale_items(sale_id);

CREATE TABLE IF NOT EXISTS payments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sale_id uuid NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
  method text NOT NULL, -- cash, pix, debit, credit, transfer, voucher
  amount numeric(12,2) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT payments_amount_positive CHECK (amount > 0)
);
CREATE INDEX IF NOT EXISTS payments_sale_idx ON payments(sale_id);

-- Finance (ledger)
CREATE TABLE IF NOT EXISTS ledger_entries (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  entry_type text NOT NULL, -- sale, sale_cancel, expense, revenue, supply, withdrawal
  sale_id uuid NULL REFERENCES sales(id) ON DELETE SET NULL,
  cash_session_id uuid NULL REFERENCES cash_sessions(id) ON DELETE SET NULL,
  amount_gross numeric(12,2) NOT NULL DEFAULT 0,
  amount_discount numeric(12,2) NOT NULL DEFAULT 0,
  amount_net numeric(12,2) NOT NULL,
  profit_estimated numeric(12,2) NOT NULL DEFAULT 0,
  notes text NULL,
  created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ledger_entries_created_idx ON ledger_entries(created_at DESC);
CREATE INDEX IF NOT EXISTS ledger_entries_type_created_idx ON ledger_entries(entry_type, created_at DESC);

-- Future: AR/AP, expenses, revenues (MVP tables kept minimal)
CREATE TABLE IF NOT EXISTS expenses (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  description text NOT NULL,
  amount numeric(12,2) NOT NULL,
  occurred_at timestamptz NOT NULL,
  created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS revenues (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  description text NOT NULL,
  amount numeric(12,2) NOT NULL,
  occurred_at timestamptz NOT NULL,
  created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS accounts_payable (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  description text NOT NULL,
  amount numeric(12,2) NOT NULL,
  due_date date NOT NULL,
  status text NOT NULL DEFAULT 'open',
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ap_status_check CHECK (status IN ('open','paid','cancelled'))
);

CREATE TABLE IF NOT EXISTS accounts_receivable (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  description text NOT NULL,
  amount numeric(12,2) NOT NULL,
  due_date date NOT NULL,
  status text NOT NULL DEFAULT 'open',
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ar_status_check CHECK (status IN ('open','received','cancelled'))
);

-- Fiscal
CREATE TABLE IF NOT EXISTS invoices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sale_id uuid NOT NULL UNIQUE REFERENCES sales(id) ON DELETE RESTRICT,
  company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
  status text NOT NULL DEFAULT 'xml_generated',
  created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS invoice_xml_files (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  invoice_id uuid NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
  file_name text NOT NULL,
  content bytea NOT NULL,
  sha256 text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS invoice_xml_files_invoice_idx ON invoice_xml_files(invoice_id, created_at DESC);

-- Audit
CREATE TABLE IF NOT EXISTS audit_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  action text NOT NULL,
  entity_type text NOT NULL,
  entity_id uuid NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  ip inet NULL,
  user_agent text NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_logs_created_idx ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_actor_created_idx ON audit_logs(actor_user_id, created_at DESC);

COMMIT;
