// Package server wires Echo HTTP server and handlers for dendrite-pulse.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/thorstenkramm/dendrite-pulse/internal/api"
	"github.com/thorstenkramm/dendrite-pulse/internal/logging"
	"github.com/thorstenkramm/dendrite-pulse/internal/ping"
)

// Config holds server settings.
type Config struct {
	Logger      *slog.Logger
	LogRequests bool
}

// Run starts the HTTP server on the given address (e.g., ":3000") and blocks until shutdown.
func Run(ctx context.Context, addr string, cfg Config) error {
	// contextcheck: base context is propagated through Echo requests; server lifecycle is controlled via ctx.
	//nolint:contextcheck
	e := buildRouter(cfg)

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
			if cfg.Logger != nil {
				cfg.Logger.Error("server shutdown error", "error", err)
			} else {
				log.Printf("server shutdown error: %v", err)
			}
		}
	}()

	if err := e.StartServer(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("start server: %w", err)
	}

	return nil
}

func buildRouter(cfg Config) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(middleware.Recover())

	if cfg.LogRequests && cfg.Logger != nil {
		e.Use(middleware.RequestID())
		e.Use(slogRequestLogger(cfg.Logger))
	} else {
		e.Use(middleware.Logger())
	}

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

// slogRequestLogger logs incoming requests with slog and stores a request-scoped logger.
func slogRequestLogger(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			rid := c.Response().Header().Get(echo.HeaderXRequestID)
			if rid == "" {
				rid = c.Request().Header.Get(echo.HeaderXRequestID)
			}

			reqLogger := logger
			if rid != "" {
				reqLogger = logger.With(slog.String("request_id", rid))
			}

			ctxWithLogger := logging.ContextWithLogger(c.Request().Context(), reqLogger)
			c.SetRequest(c.Request().WithContext(ctxWithLogger))

			// Execute handler first so c.Path() is populated with matched route
			err := next(c)

			reqLogger.DebugContext(ctxWithLogger, "new request",
				slog.String("path", c.Path()),
				slog.String("method", c.Request().Method),
				slog.String("remote_ip", c.RealIP()),
				slog.String("user_agent", c.Request().UserAgent()),
			)

			return err
		}
	}
}
