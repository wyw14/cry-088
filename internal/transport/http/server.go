package http

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/wyw14/cry-088/internal/application/analytics"
	appauth "github.com/wyw14/cry-088/internal/application/auth"
	"github.com/wyw14/cry-088/internal/application/calendar"
	appTimesheet "github.com/wyw14/cry-088/internal/application/timesheet"
	analyticsdomain "github.com/wyw14/cry-088/internal/domain/analytics"
	"github.com/wyw14/cry-088/internal/domain/organization"
	domainTimesheet "github.com/wyw14/cry-088/internal/domain/timesheet"
)

type Server struct {
	Engine     *gin.Engine
	Timesheets timesheetService
	Analytics  analyticsService
	Calendar   calendarService
	Auth       authService
	Logger     *zap.Logger
}

type timesheetService interface {
	CreateDraft(context.Context, appTimesheet.CreateDraftCommand) (domainTimesheet.Entry, error)
	BatchSubmit(context.Context, appTimesheet.BatchSubmitCommand) ([]domainTimesheet.Entry, error)
}
type analyticsService interface {
	ProjectReport(context.Context, analytics.ReportRequest) (analyticsdomain.ProjectReport, error)
}
type calendarService interface {
	PersonalMonth(context.Context, string, string, time.Time, organization.Principal) (calendar.Month, error)
}
type authService interface {
	Login(context.Context, string, string) (appauth.TokenPair, error)
	Refresh(context.Context, string) (appauth.TokenPair, error)
	ParseAccess(string) (organization.Principal, error)
}

func NewServer(logger *zap.Logger, timesheets timesheetService, analyticsServiceValue analyticsService, calendarValue calendarService, authValue authService, origins []string) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	limiter := &Limiter{Limit: 120}
	engine.Use(RequestContext(logger), Recovery(logger), SecurityHeaders(), CORS(origins), limiter.Middleware())
	server := &Server{Engine: engine, Timesheets: timesheets, Analytics: analyticsServiceValue, Calendar: calendarValue, Auth: authValue, Logger: logger}
	server.routes()
	return server
}

func (s *Server) routes() {
	s.Engine.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	s.Engine.GET("/readyz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) })
	api := s.Engine.Group("/api/v1")
	api.POST("/auth/login", s.login)
	api.POST("/auth/refresh", s.refresh)
	authorized := api.Group("", s.requireAuth)
	authorized.POST("/time-entries", s.createDraft)
	authorized.POST("/time-entries/submit", s.submit)
	authorized.GET("/projects/:projectId/report", s.report)
	authorized.GET("/calendar/:employeeId", s.calendar)
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

func (s *Server) login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}
	pair, err := s.Auth.Login(c, req.Email, req.Password)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, pair)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func (s *Server) refresh(c *gin.Context) {
	var req refreshRequest
	if !bindJSON(c, &req) {
		return
	}
	pair, err := s.Auth.Refresh(c, req.RefreshToken)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, pair)
}
func (s *Server) requireAuth(c *gin.Context) {
	header := c.GetHeader("Authorization")
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeError(c, &authError{})
		c.Abort()
		return
	}
	principal, err := s.Auth.ParseAccess(parts[1])
	if err != nil {
		writeError(c, err)
		c.Abort()
		return
	}
	c.Set("principal", principal)
	c.Next()
}

type authError struct{}

func (*authError) Error() string { return "missing bearer token" }
func principal(c *gin.Context) organization.Principal {
	value, _ := c.Get("principal")
	p, _ := value.(organization.Principal)
	return p
}

type createEntryRequest struct {
	OrganizationID string    `json:"organization_id" validate:"required"`
	EmployeeID     string    `json:"employee_id" validate:"required"`
	ProjectID      string    `json:"project_id" validate:"required"`
	TaskID         string    `json:"task_id" validate:"required"`
	WorkDate       time.Time `json:"work_date" validate:"required"`
	StartMinute    int       `json:"start_minute"`
	Minutes        int       `json:"minutes" validate:"required,min=1"`
	Description    string    `json:"description" validate:"required,min=3"`
}

func (s *Server) createDraft(c *gin.Context) {
	var req createEntryRequest
	if !bindJSON(c, &req) {
		return
	}
	entry, err := s.Timesheets.CreateDraft(c, appTimesheet.CreateDraftCommand{OrganizationID: req.OrganizationID, EmployeeID: req.EmployeeID, ProjectID: req.ProjectID, TaskID: req.TaskID, WorkDate: req.WorkDate, StartMinute: req.StartMinute, Minutes: req.Minutes, Description: req.Description, Actor: principal(c), RequestID: requestID(c)})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, entry)
}

type submitRequest struct {
	OrganizationID string   `json:"organization_id" validate:"required"`
	EmployeeID     string   `json:"employee_id" validate:"required"`
	EntryIDs       []string `json:"entry_ids" validate:"required,min=1,max=100"`
}

func (s *Server) submit(c *gin.Context) {
	var req submitRequest
	if !bindJSON(c, &req) {
		return
	}
	entries, err := s.Timesheets.BatchSubmit(c, appTimesheet.BatchSubmitCommand{OrganizationID: req.OrganizationID, EmployeeID: req.EmployeeID, EntryIDs: req.EntryIDs, Actor: principal(c), RequestID: requestID(c)})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}
func (s *Server) report(c *gin.Context) {
	p := principal(c)
	report, err := s.Analytics.ProjectReport(c, analytics.ReportRequest{OrganizationID: p.OrganizationID, ProjectID: c.Param("projectId"), AsOf: time.Now().UTC(), Actor: p})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}
func (s *Server) calendar(c *gin.Context) {
	p := principal(c)
	monthValue := c.Query("month")
	month, timeErr := time.Parse("2006-01", monthValue)
	if timeErr != nil {
		writeError(c, timeErr)
		return
	}
	result, err := s.Calendar.PersonalMonth(c, p.OrganizationID, c.Param("employeeId"), month, p)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
