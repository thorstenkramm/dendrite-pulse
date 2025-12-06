// Package ping exposes the ping endpoint.
package ping

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/thorstenkramm/dendrite-pulse/internal/api"
)

// RegisterRoutes registers ping-related routes.
func RegisterRoutes(e *echo.Echo) {
	e.GET("/api/v1/ping", handler)
}

func handler(c echo.Context) error {
	response := Response{
		Meta: PaginationMeta{
			Page: PageInfo{
				CurrentPage: 1,
				From:        1,
				LastPage:    1,
				PerPage:     1,
				To:          1,
				Total:       1,
			},
		},
		Links: PaginationLinks{
			Self:  "/api/v1/ping",
			First: "/api/v1/ping",
			Last:  "/api/v1/ping",
		},
		Data: Resource{
			Type: "ping",
			ID:   "ping",
			Attributes: Attributes{
				Message: "pong",
			},
		},
	}

	c.Response().Header().Set(echo.HeaderContentType, api.ContentType)
	if err := c.JSON(http.StatusOK, response); err != nil {
		return fmt.Errorf("write ping response: %w", err)
	}
	return nil
}
