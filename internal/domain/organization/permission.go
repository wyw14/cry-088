package organization

type Role string

const (
	RoleEmployee    Role = "employee"
	RoleProjectLead Role = "project_lead"
	RoleDepartment  Role = "department_manager"
	RoleFinance     Role = "finance"
	RoleAdmin       Role = "admin"
)

type Permission string

const (
	PermissionWriteOwnTime  Permission = "time.write.own"
	PermissionReviewTime    Permission = "time.review.project"
	PermissionManageProject Permission = "project.manage"
	PermissionViewCost      Permission = "cost.view"
	PermissionClosePeriod   Permission = "settlement.close"
	PermissionAuditRead     Permission = "audit.read"
)

var rolePermissions = map[Role]map[Permission]bool{
	RoleEmployee:    {PermissionWriteOwnTime: true},
	RoleProjectLead: {PermissionWriteOwnTime: true, PermissionReviewTime: true, PermissionManageProject: true},
	RoleDepartment:  {PermissionWriteOwnTime: true, PermissionAuditRead: true},
	RoleFinance:     {PermissionViewCost: true, PermissionClosePeriod: true, PermissionAuditRead: true},
	RoleAdmin:       {PermissionWriteOwnTime: true, PermissionReviewTime: true, PermissionManageProject: true, PermissionViewCost: true, PermissionClosePeriod: true, PermissionAuditRead: true},
}

type Principal struct {
	EmployeeID     string
	OrganizationID string
	Roles          []Role
	ProjectIDs     map[string]bool
}

func (p Principal) Has(permission Permission) bool {
	for _, role := range p.Roles {
		if rolePermissions[role][permission] {
			return true
		}
	}
	return false
}

func (p Principal) CanAccessProject(organizationID, projectID string) bool {
	if p.OrganizationID == "" || p.OrganizationID != organizationID {
		return false
	}
	for _, role := range p.Roles {
		if role == RoleAdmin || role == RoleFinance || role == RoleDepartment {
			return true
		}
	}
	return p.ProjectIDs[projectID]
}

func (p Principal) CanSeeCost(organizationID, projectID string) bool {
	return p.CanAccessProject(organizationID, projectID) && p.Has(PermissionViewCost)
}
