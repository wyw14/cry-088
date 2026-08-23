package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wyw14/cry-088/internal/application/auth"
	"github.com/wyw14/cry-088/internal/domain/audit"
	authdomain "github.com/wyw14/cry-088/internal/domain/auth"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/project"
	"github.com/wyw14/cry-088/internal/domain/settlement"
	"github.com/wyw14/cry-088/internal/domain/shared"
	"github.com/wyw14/cry-088/internal/domain/timesheet"
	platform "github.com/wyw14/cry-088/internal/platform/files"
)

type DB struct{ Pool *pgxpool.Pool }

func (r DB) q(ctx context.Context) interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
} {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return r.Pool
}

type OrganizationRepository struct{ DB DB }

func (r OrganizationRepository) Get(ctx context.Context, id string) (organization.Organization, error) {
	var result organization.Organization
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT id, name, timezone, max_daily_minutes, overtime_after_minutes, allow_overlap, version FROM organizations WHERE id=$1`, id).Scan(&result.ID, &result.Name, &result.Timezone, &result.MaxDailyMinutes, &result.OvertimeAfterMinute, &result.AllowOverlap, &result.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, shared.New(shared.CodeNotFound, "organization not found")
	}
	return result, err
}

type EmployeeRepository struct{ DB DB }

func (r EmployeeRepository) Get(ctx context.Context, id string) (organization.Employee, error) {
	var e organization.Employee
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT id, organization_id, department_id, name, email, status, hired_at, terminated_at, version FROM employees WHERE id=$1`, id).Scan(&e.ID, &e.OrganizationID, &e.DepartmentID, &e.Name, &e.Email, &e.Status, &e.HiredAt, &e.TerminatedAt, &e.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, shared.New(shared.CodeNotFound, "employee not found")
	}
	return e, err
}

type ProjectRepository struct{ DB DB }

func (r ProjectRepository) Get(ctx context.Context, id string) (project.Project, error) {
	var p project.Project
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT id, organization_id, code, name, owner_id, start_date, end_date, budget_minutes, default_cost_cents, actual_minutes, status, version FROM projects WHERE id=$1`, id).Scan(&p.ID, &p.OrganizationID, &p.Code, &p.Name, &p.OwnerID, &p.StartDate, &p.EndDate, &p.BudgetMinutes, &p.DefaultCostCents, &p.ActualMinutes, &p.Status, &p.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, shared.New(shared.CodeNotFound, "project not found")
	}
	return p, err
}
func (r ProjectRepository) GetForUpdate(ctx context.Context, id string) (project.Project, error) {
	var p project.Project
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT id, organization_id, code, name, owner_id, start_date, end_date, budget_minutes, default_cost_cents, actual_minutes, status, version FROM projects WHERE id=$1 FOR UPDATE`, id).Scan(&p.ID, &p.OrganizationID, &p.Code, &p.Name, &p.OwnerID, &p.StartDate, &p.EndDate, &p.BudgetMinutes, &p.DefaultCostCents, &p.ActualMinutes, &p.Status, &p.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, shared.New(shared.CodeNotFound, "project not found")
	}
	return p, err
}
func (r ProjectRepository) Update(ctx context.Context, p project.Project, expected int64) error {
	result, err := r.DB.q(ctx).Exec(ctx, `UPDATE projects SET actual_minutes=$1, status=$2, version=$3 WHERE id=$4 AND version=$5`, p.ActualMinutes, p.Status, p.Version, p.ID, expected)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return shared.New(shared.CodeConflict, "project version mismatch")
	}
	return nil
}

type EntryRepository struct{ DB DB }

func scanEntry(row pgx.Row) (timesheet.Entry, error) {
	var e timesheet.Entry
	err := row.Scan(&e.ID, &e.OrganizationID, &e.EmployeeID, &e.ProjectID, &e.TaskID, &e.WorkDate, &e.StartMinute, &e.Minutes, &e.Description, &e.State, &e.ReturnReason, &e.ApprovedBy, &e.ApprovedAt, &e.LockedAt, &e.ReversalID, &e.CorrectionID, &e.Version, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

const entryColumns = `id, organization_id, employee_id, project_id, task_id, work_date, start_minute, minutes, description, state, return_reason, approved_by, approved_at, locked_at, reversal_id, correction_id, version, created_at, updated_at`

func (r EntryRepository) Get(ctx context.Context, id string) (timesheet.Entry, error) {
	e, err := scanEntry(r.DB.q(ctx).QueryRow(ctx, `SELECT `+entryColumns+` FROM time_entries WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return e, shared.New(shared.CodeNotFound, "time entry not found")
	}
	return e, err
}
func (r EntryRepository) ListEmployeeDay(ctx context.Context, employeeID string, day time.Time) ([]timesheet.Entry, error) {
	return r.list(ctx, employeeID, day, day.AddDate(0, 0, 1))
}
func (r EntryRepository) ListEmployeeMonth(ctx context.Context, employeeID string, month time.Time) ([]timesheet.Entry, error) {
	start := time.Date(month.UTC().Year(), month.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	return r.list(ctx, employeeID, start, start.AddDate(0, 1, 0))
}
func (r EntryRepository) list(ctx context.Context, employeeID string, start, end time.Time) ([]timesheet.Entry, error) {
	rows, err := r.DB.q(ctx).Query(ctx, `SELECT `+entryColumns+` FROM time_entries WHERE employee_id=$1 AND work_date >= $2 AND work_date < $3 ORDER BY work_date,start_minute,id`, employeeID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []timesheet.Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
func (r EntryRepository) Insert(ctx context.Context, e timesheet.Entry) error {
	_, err := r.DB.q(ctx).Exec(ctx, `INSERT INTO time_entries (`+entryColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, e.ID, e.OrganizationID, e.EmployeeID, e.ProjectID, e.TaskID, e.WorkDate, e.StartMinute, e.Minutes, e.Description, e.State, e.ReturnReason, e.ApprovedBy, e.ApprovedAt, e.LockedAt, e.ReversalID, e.CorrectionID, e.Version, e.CreatedAt, e.UpdatedAt)
	return err
}
func (r EntryRepository) InsertMany(ctx context.Context, entries []timesheet.Entry) error {
	for _, e := range entries {
		if err := r.Insert(ctx, e); err != nil {
			return err
		}
	}
	return nil
}
func (r EntryRepository) Update(ctx context.Context, e timesheet.Entry, expected int64) error {
	result, err := r.DB.q(ctx).Exec(ctx, `UPDATE time_entries SET minutes=$1,description=$2,state=$3,return_reason=$4,approved_by=$5,approved_at=$6,locked_at=$7,reversal_id=$8,correction_id=$9,version=$10,updated_at=$11 WHERE id=$12 AND version=$13`, e.Minutes, e.Description, e.State, e.ReturnReason, e.ApprovedBy, e.ApprovedAt, e.LockedAt, e.ReversalID, e.CorrectionID, e.Version, e.UpdatedAt, e.ID, expected)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return shared.New(shared.CodeConflict, "time entry version mismatch")
	}
	return nil
}
func (r EntryRepository) ListApprovedForPeriod(ctx context.Context, organizationID string, month time.Time) ([]timesheet.Entry, error) {
	start := time.Date(month.UTC().Year(), month.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	rows, err := r.DB.q(ctx).Query(ctx, `SELECT `+entryColumns+` FROM time_entries WHERE organization_id=$1 AND work_date >= $2 AND work_date < $3 AND state=$4 ORDER BY work_date,start_minute,id`, organizationID, start, end, timesheet.EntryApproved)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []timesheet.Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

type AssignmentRepository struct{ DB DB }

func (r AssignmentRepository) ActiveFor(ctx context.Context, employeeID, projectID, taskID string, day time.Time) (project.Assignment, error) {
	var a project.Assignment
	var taskIDs []string
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT id,project_id,employee_id,role,valid_from,valid_until,task_ids,capacity_minutes,version FROM assignments WHERE employee_id=$1 AND project_id=$2 AND valid_from <= $3 AND valid_until >= $3 ORDER BY valid_from DESC LIMIT 1`, employeeID, projectID, day.UTC()).Scan(&a.ID, &a.ProjectID, &a.EmployeeID, &a.Role, &a.ValidFrom, &a.ValidUntil, &taskIDs, &a.CapacityMinutes, &a.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, shared.New(shared.CodeForbidden, "employee assignment not found")
	}
	a.TaskIDs = map[string]bool{}
	for _, id := range taskIDs {
		a.TaskIDs[id] = true
	}
	if !a.Allows(day, taskID) {
		return a, shared.New(shared.CodeForbidden, "task is outside assignment scope")
	}
	return a, err
}
func (r AssignmentRepository) ListEmployeeMonth(ctx context.Context, employeeID string, month time.Time) ([]project.Assignment, error) {
	start := time.Date(month.UTC().Year(), month.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	rows, err := r.DB.q(ctx).Query(ctx, `SELECT id,project_id,employee_id,role,valid_from,valid_until,task_ids,capacity_minutes,version FROM assignments WHERE employee_id=$1 AND valid_until >= $2 AND valid_from < $3`, employeeID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []project.Assignment{}
	for rows.Next() {
		var a project.Assignment
		var ids []string
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.EmployeeID, &a.Role, &a.ValidFrom, &a.ValidUntil, &ids, &a.CapacityMinutes, &a.Version); err != nil {
			return nil, err
		}
		a.TaskIDs = map[string]bool{}
		for _, id := range ids {
			a.TaskIDs[id] = true
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
func (r AssignmentRepository) Insert(ctx context.Context, a project.Assignment) error {
	ids := []string{}
	for id := range a.TaskIDs {
		ids = append(ids, id)
	}
	_, err := r.DB.q(ctx).Exec(ctx, `INSERT INTO assignments (id,project_id,employee_id,role,valid_from,valid_until,task_ids,capacity_minutes,version) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, a.ID, a.ProjectID, a.EmployeeID, a.Role, a.ValidFrom, a.ValidUntil, ids, a.CapacityMinutes, a.Version)
	return err
}
func (r AssignmentRepository) ReviewerCanLead(ctx context.Context, reviewerID, projectID string, day time.Time) (bool, error) {
	var found bool
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM assignments WHERE employee_id=$1 AND project_id=$2 AND role=$3 AND valid_from <= $4 AND valid_until >= $4)`, reviewerID, projectID, project.ProjectRoleLead, day.UTC()).Scan(&found)
	return found, err
}

type PeriodRepository struct{ DB DB }

func (r PeriodRepository) ForMonth(ctx context.Context, organizationID string, month time.Time) (settlement.Period, error) {
	var p settlement.Period
	start := time.Date(month.UTC().Year(), month.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT id,organization_id,month,state,closing_by,closed_by,closing_at,closed_at,version FROM settlement_periods WHERE organization_id=$1 AND month=$2`, organizationID, start).Scan(&p.ID, &p.OrganizationID, &p.Month, &p.State, &p.ClosingBy, &p.ClosedBy, &p.ClosingAt, &p.ClosedAt, &p.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, shared.New(shared.CodeNotFound, "settlement period not found")
	}
	return p, err
}
func (r PeriodRepository) GetForUpdate(ctx context.Context, id string) (settlement.Period, error) {
	var p settlement.Period
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT id,organization_id,month,state,closing_by,closed_by,closing_at,closed_at,version FROM settlement_periods WHERE id=$1 FOR UPDATE`, id).Scan(&p.ID, &p.OrganizationID, &p.Month, &p.State, &p.ClosingBy, &p.ClosedBy, &p.ClosingAt, &p.ClosedAt, &p.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, shared.New(shared.CodeNotFound, "settlement period not found")
	}
	return p, err
}
func (r PeriodRepository) Update(ctx context.Context, p settlement.Period, expected int64) error {
	result, err := r.DB.q(ctx).Exec(ctx, `UPDATE settlement_periods SET state=$1,closing_by=$2,closed_by=$3,closing_at=$4,closed_at=$5,version=$6 WHERE id=$7 AND version=$8`, p.State, p.ClosingBy, p.ClosedBy, p.ClosingAt, p.ClosedAt, p.Version, p.ID, expected)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return shared.New(shared.CodeConflict, "settlement period version mismatch")
	}
	return nil
}

type AuditRepository struct{ DB DB }

func (r AuditRepository) Append(ctx context.Context, e audit.Event) error {
	_, err := r.DB.q(ctx).Exec(ctx, `INSERT INTO audit_events (id,organization_id,actor_id,source,entity_type,entity_id,action,before_json,after_json,reason,occurred_at,request_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, e.ID, e.OrganizationID, e.ActorID, e.Source, e.EntityType, e.EntityID, e.Action, e.Before, e.After, e.Reason, e.OccurredAt, e.RequestID)
	return err
}

type SnapshotRepository struct{ DB DB }

func (r SnapshotRepository) FindByPeriod(ctx context.Context, periodID string) (settlement.CostSnapshot, bool, error) {
	var s settlement.CostSnapshot
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT id,organization_id,period_id,created_by,created_at,total_minutes,total_cents,checksum FROM cost_snapshots WHERE period_id=$1`, periodID).Scan(&s.ID, &s.OrganizationID, &s.PeriodID, &s.CreatedBy, &s.CreatedAt, &s.TotalMinutes, &s.TotalCents, &s.Checksum)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, false, nil
	}
	return s, err == nil, err
}
func (r SnapshotRepository) Insert(ctx context.Context, s settlement.CostSnapshot) error {
	_, err := r.DB.q(ctx).Exec(ctx, `INSERT INTO cost_snapshots (id,organization_id,period_id,created_by,created_at,total_minutes,total_cents,checksum) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, s.ID, s.OrganizationID, s.PeriodID, s.CreatedBy, s.CreatedAt, s.TotalMinutes, s.TotalCents, s.Checksum)
	return err
}

type AccountRepository struct{ DB DB }

func (r AccountRepository) FindByEmail(ctx context.Context, email string) (auth.Account, error) {
	var a auth.Account
	var roles []string
	var projects []string
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT employee_id,organization_id,email,password_hash,roles,project_ids,active FROM accounts WHERE email=$1`, email).Scan(&a.EmployeeID, &a.OrganizationID, &a.Email, &a.PasswordHash, &roles, &projects, &a.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, shared.New(shared.CodeNotFound, "account not found")
	}
	for _, role := range roles {
		a.Roles = append(a.Roles, organization.Role(role))
	}
	a.ProjectIDs = map[string]bool{}
	for _, id := range projects {
		a.ProjectIDs[id] = true
	}
	return a, err
}

type SessionRepository struct{ DB DB }

func (r SessionRepository) FindByTokenHash(ctx context.Context, hash string) (authdomain.RefreshSession, error) {
	var s authdomain.RefreshSession
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT id,organization_id,employee_id,token_hash,family_id,expires_at,created_at,used_at,revoked_at,replaced_by_id,version FROM refresh_sessions WHERE token_hash=$1`, hash).Scan(&s.ID, &s.OrganizationID, &s.EmployeeID, &s.TokenHash, &s.FamilyID, &s.ExpiresAt, &s.CreatedAt, &s.UsedAt, &s.RevokedAt, &s.ReplacedByID, &s.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, shared.New(shared.CodeNotFound, "refresh session not found")
	}
	return s, err
}
func (r SessionRepository) Insert(ctx context.Context, s authdomain.RefreshSession) error {
	_, err := r.DB.q(ctx).Exec(ctx, `INSERT INTO refresh_sessions (id,organization_id,employee_id,token_hash,family_id,expires_at,created_at,used_at,revoked_at,replaced_by_id,version) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, s.ID, s.OrganizationID, s.EmployeeID, s.TokenHash, s.FamilyID, s.ExpiresAt, s.CreatedAt, s.UsedAt, s.RevokedAt, s.ReplacedByID, s.Version)
	return err
}
func (r SessionRepository) Update(ctx context.Context, s authdomain.RefreshSession, expected int64) error {
	result, err := r.DB.q(ctx).Exec(ctx, `UPDATE refresh_sessions SET used_at=$1,revoked_at=$2,replaced_by_id=$3,version=$4 WHERE id=$5 AND version=$6`, s.UsedAt, s.RevokedAt, s.ReplacedByID, s.Version, s.ID, expected)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return shared.New(shared.CodeConflict, "refresh session version mismatch")
	}
	return nil
}
func (r SessionRepository) RevokeFamily(ctx context.Context, familyID string, at time.Time) error {
	_, err := r.DB.q(ctx).Exec(ctx, `UPDATE refresh_sessions SET revoked_at=$1,version=version+1 WHERE family_id=$2 AND revoked_at IS NULL`, at.UTC(), familyID)
	return err
}

type FileRepository struct{ DB DB }

func (r FileRepository) Insert(ctx context.Context, m platform.Metadata, organizationID, ownerID, projectID string) error {
	_, err := r.DB.q(ctx).Exec(ctx, `INSERT INTO file_metadata (id,name,content_type,size_bytes,digest,storage_key,organization_id,owner_id,project_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, m.ID, m.Name, m.ContentType, m.Size, m.Digest, m.StorageKey, organizationID, ownerID, projectID)
	return err
}
func (r FileRepository) Get(ctx context.Context, id string) (platform.Metadata, string, string, string, error) {
	var m platform.Metadata
	var org, owner, project string
	err := r.DB.q(ctx).QueryRow(ctx, `SELECT id,name,content_type,size_bytes,digest,storage_key,organization_id,owner_id,project_id FROM file_metadata WHERE id=$1`, id).Scan(&m.ID, &m.Name, &m.ContentType, &m.Size, &m.Digest, &m.StorageKey, &org, &owner, &project)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, "", "", "", shared.New(shared.CodeNotFound, "file metadata not found")
	}
	return m, org, owner, project, err
}
