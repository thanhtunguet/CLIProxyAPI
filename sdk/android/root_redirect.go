package android

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// managementPanelPath is where the Android build sends visitors of the root URL.
const managementPanelPath = "/management.html"

// rootRedirectMiddleware replaces the default JSON status response served at "/"
// with a 301 redirect to the management panel. Only the exact root path is
// affected; every other route is passed through untouched.
func rootRedirectMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/" && (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead) {
			c.Redirect(http.StatusMovedPermanently, managementPanelPath)
			c.Abort()
			return
		}
		c.Next()
	}
}
