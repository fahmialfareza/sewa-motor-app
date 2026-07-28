package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/newrelic/go-agent/v3/integrations/nrgin"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"
)

const (
	principalKey = "principal"
	requestIDKey = "request-id"
)

type Pinger interface {
	Ping(context.Context) error
}

type Dependencies struct {
	Repo         port.Repository
	Auth         usecase.Auth
	Users        usecase.Users
	Packages     usecase.Packages
	Transactions usecase.Transactions
	Reporting    usecase.Reporting
	Terminals    usecase.Terminals
	Sync         usecase.Sync
	Redis        Pinger
	Logger       *logrus.Logger
	NewRelic     *newrelic.Application
}

type Server struct {
	deps Dependencies
}

func New(deps Dependencies) *gin.Engine {
	server := &Server{deps: deps}
	router := gin.New()
	router.Use(
		nrgin.Middleware(deps.NewRelic),
		server.requestID(),
		server.transactionContext(),
		server.accessLog(),
		server.recovery(),
		bodyLimit(),
	)

	api := router.Group("/api/v1")
	api.GET("/health/live", server.live)
	api.GET("/health/ready", server.ready)
	api.POST("/auth/login", server.login)

	protected := api.Group("")
	protected.Use(server.authenticate())
	protected.POST("/profile/password", server.changePassword)
	protected.POST("/auth/logout", server.logout)
	protected.GET("/profile", server.profile)

	protected.GET("/users", server.listUsers)
	protected.POST("/users", server.createUser)
	protected.GET("/users/:userId", server.getUser)
	protected.PATCH("/users/:userId", server.updateUser)
	protected.DELETE("/users/:userId", server.deleteUser)
	protected.POST("/users/:userId/reset-password", server.resetPassword)

	protected.GET("/packages", server.listPackages)
	protected.POST("/packages", server.createPackage)
	protected.GET("/packages/:packageId", server.getPackage)
	protected.PATCH("/packages/:packageId", server.updatePackage)
	protected.DELETE("/packages/:packageId", server.deletePackage)

	protected.GET("/transactions", server.listTransactions)
	protected.POST("/transactions", server.createTransaction)
	protected.GET("/transactions/:transactionId", server.getTransaction)
	protected.DELETE("/transactions/:transactionId", server.deleteTransaction)
	protected.GET("/transactions/:transactionId/revisions", server.listRevisions)
	protected.POST("/transactions/:transactionId/revisions", server.correctTransaction)
	protected.GET("/transactions/:transactionId/print-attempts", server.listPrintAttempts)
	protected.POST("/transactions/:transactionId/print-attempts", server.recordPrintAttempt)

	protected.GET("/statistics/dashboard", server.dashboard)
	protected.POST("/exports/transactions", server.exportTransactions)

	protected.POST("/terminals/enroll", server.enrollTerminal)
	protected.GET("/terminals/current", server.currentTerminal)
	protected.POST("/terminals/:terminalId/revoke", server.revokeTerminal)

	protected.POST("/sync/push", server.syncPush)
	protected.GET("/sync/pull", server.syncPull)

	// Conventional deployment probes remain available outside the versioned API.
	router.GET("/healthz", server.live)
	router.GET("/readyz", server.ready)
	return router
}

func (s *Server) transactionContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		transaction := nrgin.Transaction(c)
		if transaction == nil {
			c.Next()
			return
		}
		transaction.AddAttribute("request.id", requestID(c))
		transaction.AddAttribute("http.route", c.FullPath())
		ctx := newrelic.NewContext(c.Request.Context(), transaction)
		c.Request = c.Request.WithContext(ctx)
		handlerName := c.HandlerName()
		if index := strings.LastIndex(handlerName, "."); index >= 0 {
			handlerName = handlerName[index+1:]
		}
		if handlerName == "" {
			handlerName = c.Request.Method + " " + c.FullPath()
		}
		segment := transaction.StartSegment("HTTP.Handler." + handlerName)
		defer segment.End()
		c.Next()
	}
}

func (s *Server) requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.GetHeader("X-Request-Id"))
		if err != nil {
			id = uuid.New()
		}
		value := id.String()
		c.Set(requestIDKey, value)
		c.Header("X-Request-Id", value)
		c.Next()
	}
}

func (s *Server) accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		if s.deps.Logger != nil {
			s.deps.Logger.
				WithContext(c.Request.Context()).
				WithFields(logrus.Fields{
					"request_id":  requestID(c),
					"method":      c.Request.Method,
					"path":        c.FullPath(),
					"status":      c.Writer.Status(),
					"duration_ms": time.Since(started).Milliseconds(),
					"client_ip":   c.ClientIP(),
				}).
				Info("http_request")
		}
	}
}

func (s *Server) recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("panic: %v", recovered)
				observability.NoticeError(c.Request.Context(), err, "http.panic")
				if s.deps.Logger != nil {
					s.deps.Logger.
						WithContext(c.Request.Context()).
						WithError(err).
						WithField("request_id", requestID(c)).
						Error("panic")
				}
				writeError(c, domain.NewError(domain.CodeInternal, "Terjadi kesalahan pada server"))
				c.Abort()
			}
		}()
		c.Next()
	}
}

func bodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		c.Next()
	}
}

func (s *Server) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := usecase.BearerToken(c.GetHeader("Authorization"))
		if err != nil {
			writeError(c, domain.NewError(domain.CodeUnauthorized, "Token sesi wajib disertakan"))
			c.Abort()
			return
		}
		auth, err := s.deps.Auth.Authenticate(c.Request.Context(), token)
		if err != nil {
			writeError(c, err)
			c.Abort()
			return
		}
		c.Set(principalKey, auth.Principal)
		if transaction := newrelic.FromContext(c.Request.Context()); transaction != nil {
			transaction.AddAttribute("user.id", auth.Principal.UserID.String())
			transaction.AddAttribute("user.role", string(auth.Principal.Role))
			if auth.Principal.TerminalID != nil {
				transaction.AddAttribute("terminal.id", auth.Principal.TerminalID.String())
			}
		}
		c.Next()
	}
}

func principal(c *gin.Context) domain.Principal {
	value, _ := c.Get(principalKey)
	result, _ := value.(domain.Principal)
	return result
}

func requestID(c *gin.Context) string {
	value, _ := c.Get(requestIDKey)
	result, _ := value.(string)
	return result
}

func meta(c *gin.Context) gin.H {
	return gin.H{"requestId": requestID(c), "serverTime": time.Now().UTC()}
}

func writeData(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data, "meta": meta(c)})
}

func writePage(c *gin.Context, data any, nextCursor string) {
	writePageWithMore(c, data, nextCursor, nextCursor != "")
}

func writePageWithMore(c *gin.Context, data any, nextCursor string, hasMore bool) {
	pageMeta := meta(c)
	pageMeta["hasMore"] = hasMore
	if nextCursor == "" {
		pageMeta["nextCursor"] = nil
	} else {
		pageMeta["nextCursor"] = nextCursor
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "meta": pageMeta})
}

func writeError(c *gin.Context, err error) {
	domainErr := domain.AsError(err)
	observability.NoticeError(c.Request.Context(), err, "http."+c.HandlerName())
	status := errorStatus(domainErr.Code)
	details := domainErr.Details
	if details == nil {
		details = map[string]any{}
	}
	c.JSON(status, gin.H{"error": gin.H{
		"code": domainErr.Code, "message": domainErr.Message,
		"details": details, "requestId": requestID(c),
	}})
}

func errorStatus(code string) int {
	switch code {
	case domain.CodeValidation:
		return http.StatusUnprocessableEntity
	case domain.CodeUnauthorized, domain.CodeInvalidCredentials:
		return http.StatusUnauthorized
	case domain.CodeForbidden, domain.CodePasswordChange, domain.CodeSignatureInvalid:
		return http.StatusForbidden
	case domain.CodeNotFound:
		return http.StatusNotFound
	case domain.CodeConflict, domain.CodeRevisionConflict, domain.CodeFinalSuperadmin,
		domain.CodeSelfMutation, domain.CodeIdempotencyMismatch:
		return http.StatusConflict
	case domain.CodeRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func decodeJSON(c *gin.Context, target any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.Validation("Body JSON tidak valid", map[string]any{"reason": err.Error()})
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.Validation("Body JSON hanya boleh berisi satu objek", nil)
	}
	return nil
}

func parseUUID(value, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, domain.Validation("UUID tidak valid", map[string]any{"field": field})
	}
	return id, nil
}

func (s *Server) live(c *gin.Context) {
	writeData(c, http.StatusOK, gin.H{"status": "ok", "postgres": "ok", "redis": redisStatus(c, s.deps.Redis)})
}

func (s *Server) ready(c *gin.Context) {
	postgresStatus := "ok"
	status := "ok"
	httpStatus := http.StatusOK
	if err := s.deps.Repo.Ping(c.Request.Context()); err != nil {
		observability.NoticeError(c.Request.Context(), err, "health.postgres")
		postgresStatus = "unavailable"
		status = "unavailable"
		httpStatus = http.StatusServiceUnavailable
	}
	redis := redisStatus(c, s.deps.Redis)
	if status == "ok" && redis == "unavailable" {
		status = "degraded"
	}
	writeData(c, httpStatus, gin.H{"status": status, "postgres": postgresStatus, "redis": redis})
}

func redisStatus(c *gin.Context, pinger Pinger) string {
	if pinger == nil {
		return "unavailable"
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 250*time.Millisecond)
	defer cancel()
	if err := pinger.Ping(ctx); err != nil {
		return "unavailable"
	}
	return "ok"
}
