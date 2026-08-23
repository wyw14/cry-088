package settlement

import (
	"time"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type CostLine struct {
	EmployeeID  string
	ProjectID   string
	Minutes     int
	RateCents   int64
	AmountCents int64
}

func NewCostLine(employeeID, projectID string, minutes int, rateCents int64) (CostLine, error) {
	if employeeID == "" || projectID == "" || minutes < 0 || rateCents < 0 {
		return CostLine{}, shared.New(shared.CodeInvalid, "cost line input is invalid")
	}
	amount := (int64(minutes)*rateCents + 30) / 60
	return CostLine{EmployeeID: employeeID, ProjectID: projectID, Minutes: minutes, RateCents: rateCents, AmountCents: amount}, nil
}

type CostSnapshot struct {
	ID             string
	OrganizationID string
	PeriodID       string
	CreatedBy      string
	CreatedAt      time.Time
	Lines          []CostLine
	TotalMinutes   int
	TotalCents     int64
	Checksum       string
}

func BuildSnapshot(id, organizationID, periodID, actorID string, now time.Time, lines []CostLine, checksum string) (CostSnapshot, error) {
	if id == "" || organizationID == "" || periodID == "" || actorID == "" || checksum == "" {
		return CostSnapshot{}, shared.New(shared.CodeInvalid, "cost snapshot identity is incomplete")
	}
	snapshot := CostSnapshot{ID: id, OrganizationID: organizationID, PeriodID: periodID, CreatedBy: actorID, CreatedAt: now.UTC(), Checksum: checksum, Lines: append([]CostLine(nil), lines...)}
	for _, line := range lines {
		if line.Minutes < 0 || line.RateCents < 0 || line.AmountCents < 0 {
			return CostSnapshot{}, shared.New(shared.CodeInvalid, "cost snapshot contains an invalid line")
		}
		snapshot.TotalMinutes += line.Minutes
		snapshot.TotalCents += line.AmountCents
	}
	return snapshot, nil
}
