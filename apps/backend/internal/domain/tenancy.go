package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PlatformAuditEvent struct {
	ID        uuid.UUID       `json:"id"`
	EventType string          `json:"eventType"`
	ActorID   *uuid.UUID      `json:"actorId"`
	TenantID  *uuid.UUID      `json:"tenantId"`
	Metadata  json.RawMessage `json:"metadata"`
	CreatedAt time.Time       `json:"createdAt"`
}
type OutboxOrigin struct {
	OriginSessionID uuid.UUID `json:"originSessionId"`
	OriginActorID   uuid.UUID `json:"originActorId"`
	TerminalID      uuid.UUID `json:"terminalId"`
}
type OutboxOriginValidation struct {
	OutboxOrigin
	Allowed bool `json:"allowed"`
}

type ContextKind string

const (
	ContextAccount           ContextKind = "account"
	ContextTenant            ContextKind = "tenant"
	ContextPlatform          ContextKind = "platform"
	InitialTenantIDString                = "00000000-0000-4000-8000-000000000200"
	CodeClientUpdateRequired             = "CLIENT_UPDATE_REQUIRED"
	CodeTenantSuspended                  = "TENANT_SUSPENDED"
	CodeMembershipRevoked                = "MEMBERSHIP_REVOKED"
	CodeMembershipInactive               = "MEMBERSHIP_INACTIVE"
	CodeContextRequired                  = "CONTEXT_REQUIRED"
	CodeInvitationInvalid                = "INVITATION_INVALID"
)

func InitialTenantID() uuid.UUID { return uuid.MustParse(InitialTenantIDString) }

type Tenant struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	Status          string    `json:"status"`
	ProfileRevision int       `json:"profileRevision"`
	QrisRevision    *int      `json:"qrisRevision"`
	Revision        int       `json:"revision"`
}

type TenantContext struct {
	Tenant       Tenant    `json:"tenant"`
	MembershipID uuid.UUID `json:"membershipId"`
	Role         Role      `json:"role"`
}

type AvailableContexts struct {
	Tenants                   []TenantContext `json:"tenants"`
	PlatformAdmin             bool            `json:"platformAdmin"`
	CanManageOrganization     bool            `json:"canManageOrganization"`
	TenantProvisioningEnabled bool            `json:"tenantProvisioningEnabled"`
}

type UpdateTenantInput struct {
	Name             string `json:"name"`
	ExpectedRevision int    `json:"expectedRevision"`
}

type UpdateManagedUserInput struct {
	Role   *Role `json:"role"`
	Active *bool `json:"active"`
}

type TenantMember struct {
	ID           uuid.UUID `json:"id"`
	MembershipID uuid.UUID `json:"membershipId"`
	FullName     string    `json:"fullName"`
	Username     string    `json:"username"`
	Role         Role      `json:"role"`
	Active       bool      `json:"active"`
}

type Invitation struct {
	ID           uuid.UUID  `json:"id"`
	TenantID     uuid.UUID  `json:"tenantId"`
	Role         Role       `json:"role"`
	ExpiresAt    time.Time  `json:"expiresAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	AcceptedAt   *time.Time `json:"acceptedAt"`
	RevokedAt    *time.Time `json:"revokedAt"`
	InitialOwner bool       `json:"initialOwner"`
	Code         string     `json:"code,omitempty"`
}

type TenantProfile struct {
	TenantID     uuid.UUID `json:"tenantId"`
	Revision     int       `json:"revision"`
	BusinessName string    `json:"businessName"`
	Address      string    `json:"address"`
	Phone        string    `json:"phone"`
}

type TenantQRIS struct {
	TenantID      uuid.UUID `json:"tenantId"`
	Revision      int       `json:"revision"`
	PayloadHash   string    `json:"payloadHash"`
	StaticPayload string    `json:"staticPayload"`
}

type TenantQRISConfig struct {
	Revision          int          `json:"revision"`
	ActivePayloadHash *string      `json:"activePayloadHash"`
	Payloads          []TenantQRIS `json:"payloads"`
}

type CreateTenantInput struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}
type CreateTenantResult struct {
	Tenant     Tenant     `json:"tenant"`
	Invitation Invitation `json:"invitation"`
}
type UpdateMembershipInput struct {
	Role   *Role `json:"role"`
	Active *bool `json:"active"`
}
type UpdateTenantProfileInput struct {
	ExpectedRevision int    `json:"expectedRevision"`
	BusinessName     string `json:"businessName"`
	Address          string `json:"address"`
	Phone            string `json:"phone"`
}
type UpdateTenantQRISInput struct {
	ExpectedRevision int    `json:"expectedRevision"`
	StaticPayload    string `json:"staticPayload"`
	Activate         *bool  `json:"activate,omitempty"`
}
type SwitchContextInput struct {
	Kind           ContextKind `json:"kind"`
	TenantID       uuid.UUID   `json:"tenantId"`
	InstallationID *uuid.UUID  `json:"installationId"`
}
type RegisterInvitationInput struct {
	Code     string `json:"code"`
	Username string `json:"username"`
	FullName string `json:"fullName"`
	Password string `json:"password"`
}
