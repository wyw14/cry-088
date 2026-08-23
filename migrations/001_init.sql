CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS organizations (
  id text PRIMARY KEY,
  name text NOT NULL,
  timezone text NOT NULL,
  max_daily_minutes integer NOT NULL CHECK (max_daily_minutes BETWEEN 60 AND 1440),
  overtime_after_minutes integer NOT NULL CHECK (overtime_after_minutes BETWEEN 0 AND max_daily_minutes),
  allow_overlap boolean NOT NULL DEFAULT false,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS departments (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id), parent_id text REFERENCES departments(id),
  name text NOT NULL, manager_id text, version bigint NOT NULL DEFAULT 1, UNIQUE (organization_id,name)
);
CREATE TABLE IF NOT EXISTS employees (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id), department_id text REFERENCES departments(id),
  name text NOT NULL, email text NOT NULL, status text NOT NULL CHECK (status IN ('active','inactive')), hired_at timestamptz NOT NULL,
  terminated_at timestamptz, version bigint NOT NULL DEFAULT 1, UNIQUE (organization_id,email)
);
CREATE TABLE IF NOT EXISTS projects (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id), code text NOT NULL, name text NOT NULL, owner_id text NOT NULL REFERENCES employees(id),
  start_date timestamptz NOT NULL, end_date timestamptz NOT NULL CHECK (end_date > start_date), budget_minutes integer NOT NULL CHECK (budget_minutes > 0),
  default_cost_cents bigint NOT NULL CHECK (default_cost_cents >= 0), actual_minutes integer NOT NULL DEFAULT 0 CHECK (actual_minutes >= 0),
  status text NOT NULL CHECK (status IN ('planning','active','paused','completed','archived')), version bigint NOT NULL DEFAULT 1, UNIQUE (organization_id,code)
);
CREATE TABLE IF NOT EXISTS tasks (
  id text PRIMARY KEY, project_id text NOT NULL REFERENCES projects(id), parent_id text REFERENCES tasks(id), name text NOT NULL, owner_id text REFERENCES employees(id),
  estimated_minutes integer NOT NULL CHECK (estimated_minutes > 0), actual_minutes integer NOT NULL DEFAULT 0 CHECK (actual_minutes >= 0),
  status text NOT NULL CHECK (status IN ('todo','in_progress','blocked','done')), dependency_ids text[] NOT NULL DEFAULT '{}', version bigint NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS assignments (
  id text PRIMARY KEY, project_id text NOT NULL REFERENCES projects(id), employee_id text NOT NULL REFERENCES employees(id), role text NOT NULL,
  valid_from timestamptz NOT NULL, valid_until timestamptz NOT NULL CHECK (valid_until >= valid_from), task_ids text[] NOT NULL DEFAULT '{}', capacity_minutes integer NOT NULL CHECK (capacity_minutes > 0), version bigint NOT NULL DEFAULT 1,
  UNIQUE (project_id,employee_id,valid_from)
);
CREATE INDEX IF NOT EXISTS idx_assignments_employee_range ON assignments(employee_id,valid_from,valid_until);
CREATE TABLE IF NOT EXISTS settlement_periods (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id), month date NOT NULL, state text NOT NULL CHECK (state IN ('open','closing','closed')),
  closing_by text, closed_by text, closing_at timestamptz, closed_at timestamptz, version bigint NOT NULL DEFAULT 1, UNIQUE (organization_id,month)
);
CREATE TABLE IF NOT EXISTS time_entries (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id), employee_id text NOT NULL REFERENCES employees(id), project_id text NOT NULL REFERENCES projects(id), task_id text NOT NULL REFERENCES tasks(id),
  work_date date NOT NULL, start_minute integer NOT NULL CHECK (start_minute BETWEEN 0 AND 1439), minutes integer NOT NULL CHECK (minutes BETWEEN 1 AND 1440), description text NOT NULL CHECK (length(description) BETWEEN 3 AND 1000),
  state text NOT NULL CHECK (state IN ('draft','submitted','approved','returned','locked','reversed')), return_reason text NOT NULL DEFAULT '', approved_by text, approved_at timestamptz, locked_at timestamptz,
  reversal_id text, correction_id text, version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
  UNIQUE(employee_id,work_date,start_minute,id)
);
CREATE INDEX IF NOT EXISTS idx_time_entries_employee_month ON time_entries(employee_id,work_date);
CREATE INDEX IF NOT EXISTS idx_time_entries_project_state ON time_entries(project_id,state,work_date);
CREATE TABLE IF NOT EXISTS corrections (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id), original_entry_id text NOT NULL REFERENCES time_entries(id), replacement_entry_id text NOT NULL REFERENCES time_entries(id),
  requested_by text NOT NULL REFERENCES employees(id), reason text NOT NULL, state text NOT NULL CHECK (state IN ('pending','approved','rejected')), reviewed_by text, requested_at timestamptz NOT NULL, reviewed_at timestamptz, version bigint NOT NULL DEFAULT 1,
  UNIQUE(original_entry_id,replacement_entry_id)
);
CREATE TABLE IF NOT EXISTS audit_events (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id), actor_id text NOT NULL, source text NOT NULL, entity_type text NOT NULL, entity_id text NOT NULL,
  action text NOT NULL, before_json jsonb NOT NULL, after_json jsonb NOT NULL, reason text NOT NULL, occurred_at timestamptz NOT NULL, request_id text NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_events(organization_id,entity_type,entity_id,occurred_at);
CREATE TABLE IF NOT EXISTS cost_snapshots (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id), period_id text NOT NULL REFERENCES settlement_periods(id), created_by text NOT NULL, created_at timestamptz NOT NULL,
  total_minutes integer NOT NULL CHECK (total_minutes >= 0), total_cents bigint NOT NULL CHECK (total_cents >= 0), checksum text NOT NULL, UNIQUE(period_id)
);
CREATE TABLE IF NOT EXISTS accounts (
  employee_id text PRIMARY KEY REFERENCES employees(id), organization_id text NOT NULL REFERENCES organizations(id), email text NOT NULL UNIQUE, password_hash bytea NOT NULL,
  roles text[] NOT NULL DEFAULT '{}', project_ids text[] NOT NULL DEFAULT '{}', active boolean NOT NULL DEFAULT true
);
CREATE TABLE IF NOT EXISTS refresh_sessions (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id), employee_id text NOT NULL REFERENCES employees(id), token_hash text NOT NULL UNIQUE, family_id text NOT NULL,
  expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL, used_at timestamptz, revoked_at timestamptz, replaced_by_id text, version bigint NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_refresh_family ON refresh_sessions(family_id);
CREATE TABLE IF NOT EXISTS file_metadata (
  id text PRIMARY KEY, name text NOT NULL, content_type text NOT NULL, size_bytes bigint NOT NULL CHECK (size_bytes > 0), digest text NOT NULL, storage_key text NOT NULL UNIQUE,
  organization_id text NOT NULL REFERENCES organizations(id), owner_id text NOT NULL REFERENCES employees(id), project_id text NOT NULL REFERENCES projects(id), created_at timestamptz NOT NULL DEFAULT now()
);
