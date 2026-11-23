package android

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRootRedirectMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(rootRedirectMiddleware())
	engine.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "root") })
	engine.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	tests := []struct {
		method, path string
		wantCode     int
		wantLoc      string
	}{
		{http.MethodGet, "/", http.StatusMovedPermanently, "/management.html"},
		{http.MethodHead, "/", http.StatusMovedPermanently, "/management.html"},
		{http.MethodGet, "/healthz", http.StatusOK, ""},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
		if w.Code != tt.wantCode || w.Header().Get("Location") != tt.wantLoc {
			t.Errorf("%s %s: got %d Location=%q, want %d %q", tt.method, tt.path, w.Code, w.Header().Get("Location"), tt.wantCode, tt.wantLoc)
		}
	}
}
