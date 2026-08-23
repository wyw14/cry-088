package http

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http/httptest"
	"testing"
)

func TestHealthAndSecurityHeaders(t *testing.T) {
	server := NewServer(zap.NewNop(), nil, nil, nil, nil, []string{"http://localhost:5173"})
	request := httptest.NewRequest("GET", "/healthz", nil)
	response := httptest.NewRecorder()
	server.Engine.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("status=%d", response.Code)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security header missing")
	}
	_ = gin.Mode
}
