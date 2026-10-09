package middleware

import (
	"strings"

	"authway/apps/central/api/pkg/tenantscope"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// corsAllowHeaders lists every request header a browser caller may send. A
// header the API reads but this list omits fails the preflight, and the
// browser then drops the request before it reaches a handler.
var corsAllowHeaders = []string{
	"Origin",
	"Content-Type",
	"Accept",
	"Authorization",
	tenantscope.Header,
	"Request-Id",
	"Traceparent",
	"Tracestate",
}

// CORS admits cross-origin requests from the given origins, with credentials.
func CORS(allowedOrigins []string) fiber.Handler {
	return cors.New(cors.Config{
		AllowOrigins:     strings.Join(allowedOrigins, ","),
		AllowMethods:     "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders:     strings.Join(corsAllowHeaders, ","),
		AllowCredentials: true,
	})
}
