package organization

import (
	"strings"
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type EmployeeStatus string

const (
	EmployeeActive   EmployeeStatus = "active"
	EmployeeInactive EmployeeStatus = "inactive"
)

type Employee struct {
	ID             string
	OrganizationID string
	DepartmentID   string
	Name           string
	Email          string
	Status         EmployeeStatus
	HiredAt        time.Time
	TerminatedAt   *time.Time
	Version        int64
}

func NewEmployee(id, organizationID, departmentID, name, email string, hiredAt time.Time) (*Employee, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(organizationID) == "" || strings.TrimSpace(name) == "" {
		return nil, shared.New(shared.CodeInvalid, "employee identity is incomplete")
	}
	if !strings.Contains(email, "@") {
		return nil, shared.Field(shared.CodeInvalid, "employee email is invalid", "email", "invalid address")
	}
	return &Employee{ID: id, OrganizationID: organizationID, DepartmentID: departmentID, Name: name, Email: strings.ToLower(email), Status: EmployeeActive, HiredAt: hiredAt.UTC(), Version: 1}, nil
}

func (e Employee) ActiveOn(day time.Time) bool {
	day = day.UTC()
	if e.Status != EmployeeActive || day.Before(e.HiredAt) {
		return false
	}
	return e.TerminatedAt == nil || day.Before(e.TerminatedAt.UTC())
}

type Department struct {
	ID             string
	OrganizationID string
	ParentID       string
	Name           string
	ManagerID      string
	Version        int64
}

func (d Department) Validate() error {
	if d.ID == "" || d.OrganizationID == "" || strings.TrimSpace(d.Name) == "" {
		return shared.New(shared.CodeInvalid, "department identity is incomplete")
	}
	if d.ID == d.ParentID {
		return shared.Field(shared.CodeInvalid, "department cannot be its own parent", "parent_id", "cycle")
	}
	return nil
}
