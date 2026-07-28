package domain

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ulidPattern = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

type LoginInput struct {
	Username       string
	Password       string
	InstallationID *uuid.UUID
	IPAddress      string
}

type LoginResult struct {
	Token     string    `json:"token"`
	Principal Principal `json:"user"`
}

type CreateUserInput struct {
	FullName          string
	Username          string
	Role              Role
	TemporaryPassword string
}

type UpdateUserInput struct {
	FullName *string
	Username *string
	Role     *Role
	IsActive *bool
}

type EnrollTerminalInput struct {
	InstallationID string
	Name           string
	PublicKey      []byte
	DeviceModel    *string
	OSVersion      *string
	AppVersion     *string
}

type CreatePackageInput struct {
	Code         string
	Name         string
	Description  string
	UnitPrice    int64
	ChangeReason string
}

type UpdatePackageInput struct {
	Name         string
	Description  string
	UnitPrice    int64
	ChangeReason string
}

type ItemInput struct {
	PackageID       uuid.UUID `json:"packageId"`
	PackageRevision int       `json:"packageRevision"`
	Quantity        int       `json:"quantity"`
}

type MutationIdentity struct {
	OriginActorID        uuid.UUID
	OriginSessionID      uuid.UUID
	TerminalID           *uuid.UUID
	SubmittedByActorID   uuid.UUID
	SubmittedBySessionID uuid.UUID
}

type CreateTransactionInput struct {
	ID         string      `json:"id"`
	OccurredAt time.Time   `json:"occurredAt"`
	Items      []ItemInput `json:"items"`
	Identity   MutationIdentity
}

type CorrectTransactionInput struct {
	ID           string      `json:"-"`
	BaseRevision int         `json:"baseRevision"`
	Reason       string      `json:"reason"`
	OccurredAt   time.Time   `json:"occurredAt"`
	Items        []ItemInput `json:"items"`
	Identity     MutationIdentity
}

type PrintAttemptInput struct {
	ID                uuid.UUID       `json:"id"`
	TransactionID     string          `json:"transactionId,omitempty"`
	Revision          int             `json:"transactionRevision"`
	Status            string          `json:"status"`
	IsCopy            bool            `json:"isCopy"`
	PrinterKind       string          `json:"printerKind"`
	PrinterIdentifier *string         `json:"printerIdentifier,omitempty"`
	ErrorCode         *string         `json:"errorCode,omitempty"`
	ErrorMessage      *string         `json:"errorMessage,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
	OccurredAt        time.Time       `json:"occurredAt"`
	Identity          MutationIdentity
}

type TransactionFilter struct {
	Search         string
	From           *time.Time
	To             *time.Time
	PackageID      *uuid.UUID
	CreatorID      *uuid.UUID
	TerminalID     *uuid.UUID
	IncludeDeleted bool
	Limit          int
	CursorOccurred *time.Time
	CursorID       string
}

type TransactionPage struct {
	Transactions []Transaction `json:"items"`
	NextCursor   string        `json:"nextCursor,omitempty"`
}

type Cursor struct {
	OccurredAt time.Time `json:"occurredAt"`
	ID         string    `json:"id"`
}

func EncodeCursor(cursor Cursor) string {
	body, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(body)
}

func DecodeCursor(value string) (Cursor, error) {
	var cursor Cursor
	body, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return cursor, err
	}
	err = json.Unmarshal(body, &cursor)
	return cursor, err
}

func NormalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func NormalizePackageCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func ValidatePassword(value string) error {
	if len(value) < 12 || len(value) > 256 {
		return Validation("Kata sandi harus terdiri dari 12–256 karakter", map[string]any{"field": "password"})
	}
	return nil
}

func ValidateItems(items []ItemInput) error {
	if len(items) == 0 {
		return Validation("Transaksi harus memiliki setidaknya satu paket", map[string]any{"field": "items"})
	}
	if len(items) > 100 {
		return Validation("Transaksi memiliki terlalu banyak baris", map[string]any{"field": "items", "maximum": 100})
	}
	type packageRevisionKey struct {
		PackageID uuid.UUID
		Revision  int
	}
	seen := make(map[packageRevisionKey]struct{}, len(items))
	for index, item := range items {
		if item.PackageID == uuid.Nil || item.PackageRevision < 1 {
			return Validation("Referensi paket tidak valid", map[string]any{"field": "items", "index": index})
		}
		if item.Quantity < 1 || item.Quantity > 999 {
			return Validation("Jumlah paket harus antara 1 dan 999", map[string]any{"field": "items.quantity", "index": index})
		}
		key := packageRevisionKey{PackageID: item.PackageID, Revision: item.PackageRevision}
		if _, ok := seen[key]; ok {
			return Validation("Paket yang sama tidak boleh muncul dua kali", map[string]any{"field": "items", "index": index})
		}
		seen[key] = struct{}{}
	}
	return nil
}

func ValidateTransactionID(value string) error {
	if !ulidPattern.MatchString(value) {
		return Validation("ID transaksi harus berupa ULID 26 karakter", map[string]any{"field": "id"})
	}
	return nil
}

func CheckedLineTotal(price int64, quantity int) (int64, error) {
	if price <= 0 || quantity < 1 || quantity > 999 || price > math.MaxInt64/int64(quantity) {
		return 0, Validation("Nilai item transaksi tidak valid", nil)
	}
	return price * int64(quantity), nil
}

type SyncMutation struct {
	OperationID     string          `json:"operationId"`
	Aggregate       string          `json:"aggregate"`
	Action          string          `json:"action"`
	AggregateID     string          `json:"aggregateId"`
	BaseRevision    *int            `json:"baseRevision,omitempty"`
	OriginSessionID uuid.UUID       `json:"originSessionId"`
	OriginActorID   uuid.UUID       `json:"originActorId"`
	TerminalID      uuid.UUID       `json:"terminalId"`
	OccurredAt      time.Time       `json:"occurredAt"`
	Payload         json.RawMessage `json:"payload"`
	Signature       string          `json:"signature"`
}

type SyncOperationResult struct {
	OperationID string          `json:"operationId"`
	Status      int             `json:"status"`
	Data        json.RawMessage `json:"data,omitempty"`
	Error       *Error          `json:"error,omitempty"`
	Replayed    bool            `json:"replayed"`
}
