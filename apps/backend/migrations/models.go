package migrations

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type schemaMigration struct {
	Version   string    `gorm:"type:text;primaryKey"`
	AppliedAt time.Time `gorm:"type:timestamptz;not null;default:now()"`
}

func (schemaMigration) TableName() string { return "schema_migrations" }

type userModel struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	FullName           string     `gorm:"type:text;not null;check:users_full_name_length,length(btrim(full_name)) BETWEEN 1 AND 160"`
	Username           string     `gorm:"type:text;not null;uniqueIndex:users_username_key;check:users_username_format,username = lower(username) AND username ~ '^[a-z0-9][a-z0-9._-]{2,63}$'"`
	PasswordHash       string     `gorm:"type:text;not null"`
	Role               string     `gorm:"type:text;not null;index:users_active_role_idx,where:is_active AND deleted_at IS NULL;check:users_role_allowed,role IN ('admin','superadmin')"`
	IsActive           bool       `gorm:"not null;default:true"`
	MustChangePassword bool       `gorm:"not null;default:true"`
	CreatedAt          time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt          time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	DeletedAt          *time.Time `gorm:"type:timestamptz;check:users_deleted_inactive,deleted_at IS NULL OR NOT is_active"`
}

func (userModel) TableName() string { return "users" }

type terminalModel struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	InstallationID string     `gorm:"type:text;not null;uniqueIndex:terminals_installation_id_key;check:terminals_installation_id_length,length(installation_id) BETWEEN 8 AND 200"`
	Name           string     `gorm:"type:text;not null;check:terminals_name_length,length(btrim(name)) BETWEEN 1 AND 120"`
	PublicKey      []byte     `gorm:"type:bytea;not null;check:terminals_public_key_length,octet_length(public_key) = 32"`
	Platform       string     `gorm:"type:text;not null;default:'android';check:terminals_android_only,platform = 'android'"`
	DeviceModel    *string    `gorm:"type:text"`
	OSVersion      *string    `gorm:"type:text"`
	AppVersion     *string    `gorm:"type:text"`
	IsActive       bool       `gorm:"not null;default:true"`
	EnrolledBy     uuid.UUID  `gorm:"type:uuid;not null"`
	CreatedAt      time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt      time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	RevokedAt      *time.Time `gorm:"type:timestamptz;check:terminals_revoked_inactive,revoked_at IS NULL OR NOT is_active"`
}

func (terminalModel) TableName() string { return "terminals" }

type sessionModel struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID        uuid.UUID  `gorm:"type:uuid;not null;index:sessions_user_live_idx,where:revoked_at IS NULL"`
	TerminalID    *uuid.UUID `gorm:"type:uuid"`
	TokenHash     []byte     `gorm:"type:bytea;not null;uniqueIndex:sessions_token_hash_key;check:sessions_token_hash_length,octet_length(token_hash) = 32"`
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	LastSeenAt    time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	RevokedAt     *time.Time `gorm:"type:timestamptz;check:sessions_revocation_reason,(revoked_at IS NULL AND revoked_reason IS NULL) OR (revoked_at IS NOT NULL AND length(btrim(revoked_reason)) > 0)"`
	RevokedReason *string    `gorm:"type:text"`
}

func (sessionModel) TableName() string { return "sessions" }

type packageModel struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Code            string     `gorm:"type:text;not null;uniqueIndex:packages_code_key;check:packages_code_format,code ~ '^[A-Z0-9][A-Z0-9_-]{1,31}$'"`
	CurrentRevision int        `gorm:"type:integer;not null;default:1;check:packages_current_revision_positive,current_revision > 0"`
	CreatedBy       *uuid.UUID `gorm:"type:uuid"`
	UpdatedBy       *uuid.UUID `gorm:"type:uuid"`
	CreatedAt       time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt       time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	DeletedAt       *time.Time `gorm:"type:timestamptz"`
	DeletedBy       *uuid.UUID `gorm:"type:uuid"`
}

func (packageModel) TableName() string { return "packages" }

type packageRevisionModel struct {
	PackageID    uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Revision     int        `gorm:"type:integer;primaryKey;autoIncrement:false;check:package_revisions_revision_positive,revision > 0"`
	Name         string     `gorm:"type:text;not null;check:package_revisions_name_length,length(btrim(name)) BETWEEN 1 AND 120"`
	Description  string     `gorm:"type:text;not null;default:'';check:package_revisions_description_length,length(description) <= 1000"`
	UnitPrice    int64      `gorm:"type:bigint;not null;check:package_revisions_unit_price_positive,unit_price > 0"`
	ChangeReason *string    `gorm:"type:text"`
	CreatedBy    *uuid.UUID `gorm:"type:uuid"`
	CreatedAt    time.Time  `gorm:"type:timestamptz;not null;default:now()"`
}

func (packageRevisionModel) TableName() string { return "package_revisions" }

type transactionModel struct {
	ID                    string     `gorm:"type:text;primaryKey;index:transactions_occurred_idx,priority:2,sort:desc;check:transactions_id_ulid,id ~ '^[0-9A-HJKMNP-TV-Z]{26}$'"`
	CurrentRevision       int        `gorm:"type:integer;not null;default:1;check:transactions_current_revision_positive,current_revision > 0"`
	OccurredAt            time.Time  `gorm:"type:timestamptz;not null;index:transactions_occurred_idx,priority:1,sort:desc;index:transactions_origin_actor_idx,priority:2,sort:desc;index:transactions_live_idx,sort:desc,where:deleted_at IS NULL"`
	ServerReceivedAt      time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	OriginActorID         uuid.UUID  `gorm:"type:uuid;not null;index:transactions_origin_actor_idx,priority:1"`
	OriginSessionID       uuid.UUID  `gorm:"type:uuid;not null"`
	TerminalID            *uuid.UUID `gorm:"type:uuid"`
	UpdatedBy             uuid.UUID  `gorm:"type:uuid;not null"`
	Subtotal              int64      `gorm:"type:bigint;not null;check:transactions_subtotal_nonnegative,subtotal >= 0"`
	Total                 int64      `gorm:"type:bigint;not null;check:transactions_total_matches_subtotal,total >= 0 AND total = subtotal"`
	PrintState            string     `gorm:"type:text;not null;default:'pending';check:transactions_print_state_allowed,print_state IN ('pending','success','failed','unknown','needs-reprint')"`
	LatestPrintedRevision *int       `gorm:"type:integer"`
	UpdatedAt             time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	DeletedAt             *time.Time `gorm:"type:timestamptz;check:transactions_deletion_metadata,(deleted_at IS NULL AND deleted_by IS NULL AND delete_reason IS NULL) OR (deleted_at IS NOT NULL AND deleted_by IS NOT NULL AND length(btrim(delete_reason)) > 0)"`
	DeletedBy             *uuid.UUID `gorm:"type:uuid"`
	DeleteReason          *string    `gorm:"type:text"`
}

func (transactionModel) TableName() string { return "transactions" }

type transactionRevisionModel struct {
	TransactionID        string          `gorm:"type:text;primaryKey"`
	Revision             int             `gorm:"type:integer;primaryKey;autoIncrement:false;check:transaction_revisions_revision_positive,revision > 0"`
	BaseRevision         *int            `gorm:"type:integer;check:transaction_revisions_base_revision_positive,base_revision IS NULL OR base_revision > 0"`
	ChangeType           string          `gorm:"type:text;not null;check:transaction_revisions_change_type_allowed,change_type IN ('create','correction')"`
	Reason               *string         `gorm:"type:text"`
	BeforeSnapshot       json.RawMessage `gorm:"type:jsonb;check:transaction_revisions_shape,(change_type = 'create' AND revision = 1 AND base_revision IS NULL AND reason IS NULL AND before_snapshot IS NULL) OR (change_type = 'correction' AND revision > 1 AND base_revision = revision - 1 AND length(btrim(reason)) >= 5 AND before_snapshot IS NOT NULL)"`
	AfterSnapshot        json.RawMessage `gorm:"type:jsonb;not null"`
	OriginActorID        uuid.UUID       `gorm:"type:uuid;not null"`
	OriginSessionID      uuid.UUID       `gorm:"type:uuid;not null"`
	TerminalID           *uuid.UUID      `gorm:"type:uuid"`
	SubmittedByActorID   uuid.UUID       `gorm:"type:uuid;not null"`
	SubmittedBySessionID uuid.UUID       `gorm:"type:uuid;not null"`
	ClientOccurredAt     time.Time       `gorm:"type:timestamptz;not null"`
	ServerReceivedAt     time.Time       `gorm:"type:timestamptz;not null;default:now()"`
}

func (transactionRevisionModel) TableName() string { return "transaction_revisions" }

type transactionItemModel struct {
	TransactionID      string    `gorm:"type:text;primaryKey;index:transaction_items_current_filter_idx,priority:2"`
	Revision           int       `gorm:"type:integer;primaryKey;autoIncrement:false;index:transaction_items_current_filter_idx,priority:3"`
	LineNumber         int       `gorm:"type:integer;primaryKey;autoIncrement:false;check:transaction_items_line_number_positive,line_number > 0"`
	PackageID          uuid.UUID `gorm:"type:uuid;not null;index:transaction_items_current_filter_idx,priority:1"`
	PackageRevision    int       `gorm:"type:integer;not null"`
	PackageCode        string    `gorm:"type:text;not null"`
	PackageName        string    `gorm:"type:text;not null"`
	PackageDescription string    `gorm:"type:text;not null;default:''"`
	UnitPrice          int64     `gorm:"type:bigint;not null;check:transaction_items_unit_price_positive,unit_price > 0"`
	Quantity           int       `gorm:"type:integer;not null;check:transaction_items_quantity_range,quantity BETWEEN 1 AND 999"`
	LineTotal          int64     `gorm:"type:bigint;not null;check:transaction_items_line_total,line_total = unit_price * quantity"`
}

func (transactionItemModel) TableName() string { return "transaction_items" }

type printAttemptModel struct {
	ID                  uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TransactionID       string          `gorm:"type:text;not null;index:print_attempts_transaction_idx,priority:1"`
	TransactionRevision int             `gorm:"type:integer;not null"`
	TerminalID          *uuid.UUID      `gorm:"type:uuid"`
	ActorID             uuid.UUID       `gorm:"type:uuid;not null"`
	SessionID           uuid.UUID       `gorm:"type:uuid;not null"`
	Status              string          `gorm:"type:text;not null;check:print_attempts_status_allowed,status IN ('pending','success','failed','unknown')"`
	IsCopy              bool            `gorm:"not null;default:false"`
	PrinterKind         string          `gorm:"type:text;not null;check:print_attempts_printer_kind_allowed,printer_kind IN ('simulator','bluetooth','integrated')"`
	PrinterIdentifier   *string         `gorm:"type:text"`
	ErrorCode           *string         `gorm:"type:text;check:print_attempts_error_shape,(status IN ('failed','unknown')) OR (error_code IS NULL AND error_message IS NULL)"`
	ErrorMessage        *string         `gorm:"type:text"`
	Metadata            json.RawMessage `gorm:"type:jsonb;not null;default:'{}'::jsonb"`
	ClientOccurredAt    time.Time       `gorm:"type:timestamptz;not null"`
	ServerReceivedAt    time.Time       `gorm:"type:timestamptz;not null;default:now();index:print_attempts_transaction_idx,priority:2,sort:desc"`
}

func (printAttemptModel) TableName() string { return "print_attempts" }

type auditEventModel struct {
	ID                   uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	EventType            string          `gorm:"type:text;not null"`
	AggregateType        string          `gorm:"type:text;not null;index:audit_events_aggregate_idx,priority:1"`
	AggregateID          string          `gorm:"type:text;not null;index:audit_events_aggregate_idx,priority:2"`
	OriginActorID        *uuid.UUID      `gorm:"type:uuid"`
	OriginSessionID      *uuid.UUID      `gorm:"type:uuid"`
	SubmittedByActorID   *uuid.UUID      `gorm:"type:uuid;index:audit_events_actor_idx,priority:1"`
	SubmittedBySessionID *uuid.UUID      `gorm:"type:uuid"`
	TerminalID           *uuid.UUID      `gorm:"type:uuid"`
	BeforeValues         json.RawMessage `gorm:"type:jsonb"`
	AfterValues          json.RawMessage `gorm:"type:jsonb"`
	Metadata             json.RawMessage `gorm:"type:jsonb;not null;default:'{}'::jsonb"`
	OccurredAt           time.Time       `gorm:"type:timestamptz;not null"`
	ServerReceivedAt     time.Time       `gorm:"type:timestamptz;not null;default:now();index:audit_events_aggregate_idx,priority:3,sort:desc;index:audit_events_actor_idx,priority:2,sort:desc"`
}

func (auditEventModel) TableName() string { return "audit_events" }

type syncChangeModel struct {
	Cursor      int64           `gorm:"type:bigint GENERATED ALWAYS AS IDENTITY;primaryKey;autoIncrement:false;index:sync_changes_aggregate_idx,priority:3,sort:desc"`
	Aggregate   string          `gorm:"type:text;not null;index:sync_changes_aggregate_idx,priority:1;check:sync_changes_aggregate_allowed,aggregate IN ('user','package','transaction','print_attempt','terminal')"`
	AggregateID string          `gorm:"type:text;not null;index:sync_changes_aggregate_idx,priority:2"`
	Action      string          `gorm:"type:text;not null;check:sync_changes_action_allowed,action IN ('created','updated','deleted')"`
	Revision    *int            `gorm:"type:integer"`
	Payload     json.RawMessage `gorm:"type:jsonb;not null"`
	Tombstone   bool            `gorm:"not null;default:false;check:sync_changes_tombstone_action,(tombstone AND action = 'deleted') OR NOT tombstone"`
	CreatedAt   time.Time       `gorm:"type:timestamptz;not null;default:now()"`
}

func (syncChangeModel) TableName() string { return "sync_changes" }

type idempotencyRecordModel struct {
	TerminalID     uuid.UUID       `gorm:"type:uuid;primaryKey"`
	OperationID    string          `gorm:"type:text;primaryKey;check:idempotency_records_operation_id_length,length(operation_id) BETWEEN 8 AND 100"`
	RequestHash    []byte          `gorm:"type:bytea;not null;check:idempotency_records_request_hash_length,octet_length(request_hash) = 32"`
	ResponseStatus int             `gorm:"type:integer;not null;check:idempotency_records_response_status_range,response_status BETWEEN 100 AND 599"`
	Response       json.RawMessage `gorm:"type:jsonb;not null"`
	CreatedAt      time.Time       `gorm:"type:timestamptz;not null;default:now()"`
}

func (idempotencyRecordModel) TableName() string { return "idempotency_records" }

func initialSchemaModels() []any {
	return []any{
		&userModel{},
		&terminalModel{},
		&sessionModel{},
		&packageModel{},
		&packageRevisionModel{},
		&transactionModel{},
		&transactionRevisionModel{},
		&transactionItemModel{},
		&printAttemptModel{},
		&auditEventModel{},
		&syncChangeModel{},
		&idempotencyRecordModel{},
	}
}
