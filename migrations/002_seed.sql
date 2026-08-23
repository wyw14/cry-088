INSERT INTO organizations (id,name,timezone,max_daily_minutes,overtime_after_minutes) VALUES ('org-demo','Demo Studio','Asia/Shanghai',720,480) ON CONFLICT (id) DO NOTHING;
INSERT INTO departments (id,organization_id,name) VALUES ('dept-demo','org-demo','Delivery') ON CONFLICT (id) DO NOTHING;
INSERT INTO employees (id,organization_id,department_id,name,email,status,hired_at) VALUES ('emp-demo','org-demo','dept-demo','演示员工','demo@example.com','active',now()) ON CONFLICT (id) DO NOTHING;
INSERT INTO projects (id,organization_id,code,name,owner_id,start_date,end_date,budget_minutes,default_cost_cents,status) VALUES ('project-demo','org-demo','DEMO-001','工时闭环演示','emp-demo',now()-interval '30 days',now()+interval '60 days',9600,18000,'active') ON CONFLICT (id) DO NOTHING;
INSERT INTO settlement_periods (id,organization_id,month,state) VALUES ('period-demo','org-demo',date_trunc('month',now())::date,'open') ON CONFLICT (id) DO NOTHING;
