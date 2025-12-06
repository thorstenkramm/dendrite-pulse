// Package server wires Echo HTTP server and handlers for dendrite-pulse.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/thorstenkramm/dendrite-pulse/internal/api"
	"github.com/thorstenkramm/dendrite-pulse/internal/ping"
)

// Run starts the HTTP server on the given address (e.g., ":3000") and blocks until shutdown.
func Run(ctx context.Context, addr string) error {
	e := buildRouter()

	srv := &http.Server{
		Addr:    addr,
		Handler: e,
		// Guard against slowloris attacks.
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	}()

	if err := e.StartServer(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("start server: %w", err)
	}

	return nil
}

func buildRouter() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	e.HTTPErrorHandler = jsonAPIErrorHandler

	ping.RegisterRoutes(e)

	return e
}

func jsonAPIErrorHandler(err error, c echo.Context) {
	code := http.StatusInternalServerError
	detail := "An unexpected error occurred."

	var httpErr *echo.HTTPError
	if errors.As(err, &httpErr) {
		code = httpErr.Code
		if msg, ok := httpErr.Message.(string); ok && msg != "" {
			detail = msg
		} else {
			detail = http.StatusText(code)
		}
	}

	payload := ErrorResponse{
		Errors: []ErrorObject{
			{
				Status: fmt.Sprintf("%d", code),
				Title:  http.StatusText(code),
				Detail: detail,
			},
		},
	}

	if !c.Response().Committed {
		c.Response().Header().Set(echo.HeaderContentType, api.ContentType)
		_ = c.JSON(code, payload)
	}
}
