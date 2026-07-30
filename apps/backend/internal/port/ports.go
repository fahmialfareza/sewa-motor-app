package port

import (
	"context"
	"encoding/json"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

type Clock interface {
	Now() time.Time
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encoded string) (bool, error)
}

type TokenManager interface {
	New() (raw string, hash []byte, err error)
	Hash(raw string) ([]byte, error)
}

// SessionIndex may only cache the immutable token-hash -> session-ID mapping.
// Authorization state is deliberately re-read from PostgreSQL on every request.
type SessionIndex interface {
	Get(ctx context.Context, tokenHash []byte) (uuid.UUID, bool)
	Set(ctx context.Context, tokenHash []byte, sessionID uuid.UUID)
	Delete(ctx context.Context, tokenHash []byte)
}

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

type Repository interface {
	Ping(ctx context.Context) error

	UserForLogin(ctx context.Context, username string) (domain.UserAuth, error)
	TerminalIDByInstallation(ctx context.Context, installationID uuid.UUID) (*uuid.UUID, error)
	CreateSession(ctx context.Context, userID uuid.UUID, terminalID *uuid.UUID, tokenHash []byte) (domain.Principal, error)
	PrincipalByTokenHash(ctx context.Context, tokenHash []byte) (domain.Principal, error)
	PrincipalBySession(ctx context.Context, sessionID uuid.UUID, tokenHash []byte) (domain.Principal, error)
	RevokeSession(ctx context.Context, sessionID, actorID uuid.UUID, reason string) error
	ChangeOwnPassword(ctx context.Context, principal domain.Principal, passwordHash string) error

	ListUsers(ctx context.Context, includeDeleted bool) ([]domain.User, error)
	GetUser(ctx context.Context, id uuid.UUID) (domain.User, error)
	CreateUser(ctx context.Context, actor domain.Principal, input domain.CreateUserInput, passwordHash string) (domain.User, error)
	UpdateUser(ctx context.Context, actor domain.Principal, targetID uuid.UUID, input domain.UpdateUserInput) (domain.User, error)
	ResetUserPassword(ctx context.Context, actor domain.Principal, targetID uuid.UUID, passwordHash string) (domain.User, error)
	DeleteUser(ctx context.Context, actor domain.Principal, targetID uuid.UUID, reason string) error

	ListPackages(ctx context.Context, includeDeleted bool) ([]domain.Package, error)
	GetPackage(ctx context.Context, id uuid.UUID) (domain.Package, error)
	CreatePackage(ctx context.Context, actor domain.Principal, input domain.CreatePackageInput) (domain.Package, error)
	UpdatePackage(ctx context.Context, actor domain.Principal, id uuid.UUID, input domain.UpdatePackageInput) (domain.Package, error)
	DeletePackage(ctx context.Context, actor domain.Principal, id uuid.UUID, reason string) error

	CreateTransaction(ctx context.Context, input domain.CreateTransactionInput) (domain.Transaction, error)
	CorrectTransaction(ctx context.Context, input domain.CorrectTransactionInput) (domain.Transaction, error)
	SetTransactionPaymentStatus(ctx context.Context, input domain.SetPaymentStatusInput) (domain.Transaction, error)
	GetTransaction(ctx context.Context, id string, includeDeleted bool) (domain.Transaction, error)
	ListTransactions(ctx context.Context, filter domain.TransactionFilter) (domain.TransactionPage, error)
	DeleteTransaction(ctx context.Context, actor domain.Principal, id, reason string) error
	RecordPrintAttempt(ctx context.Context, input domain.PrintAttemptInput) (domain.PrintAttempt, error)
	ListTransactionRevisions(ctx context.Context, id string) ([]domain.TransactionRevision, error)
	ListPrintAttempts(ctx context.Context, id string) ([]domain.PrintAttempt, error)

	Dashboard(ctx context.Context, from, to time.Time, bucket string) (domain.Dashboard, error)
	ExportRows(ctx context.Context, filter domain.TransactionFilter) ([]domain.ExportRow, error)

	EnrollTerminal(ctx context.Context, principal domain.Principal, input domain.EnrollTerminalInput) (domain.Terminal, error)
	GetTerminal(ctx context.Context, id uuid.UUID) (domain.Terminal, error)
	RevokeTerminal(ctx context.Context, principal domain.Principal, id uuid.UUID) (domain.Terminal, error)
	TerminalPublicKey(ctx context.Context, terminalID uuid.UUID) ([]byte, error)
	OriginSessionMatches(ctx context.Context, sessionID, actorID, terminalID uuid.UUID) (bool, error)
	PullChanges(ctx context.Context, cursor int64, limit int) ([]domain.SyncChange, error)
	GetOperationResult(ctx context.Context, terminalID uuid.UUID, operationID string) (*domain.StoredOperationResult, error)
	StoreOperationResult(ctx context.Context, terminalID uuid.UUID, operationID string, requestHash []byte, status int, response json.RawMessage) error
	ApplySyncMutation(ctx context.Context, submitter domain.Principal, operation domain.SyncMutation, requestHash []byte) (result domain.StoredOperationResult, replayed bool, operationErr error)
}

type Exporter interface {
	XLSX(rows []domain.ExportRow, from, to *time.Time) ([]byte, error)
	PDF(rows []domain.ExportRow, from, to *time.Time) ([]byte, error)
}
