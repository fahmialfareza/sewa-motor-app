package postgres

import (
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

type userRecord struct {
	ID                 uuid.UUID   `gorm:"column:id;type:uuid;primaryKey"`
	FullName           string      `gorm:"column:full_name"`
	Username           string      `gorm:"column:username"`
	PasswordHash       string      `gorm:"column:password_hash"`
	Role               domain.Role `gorm:"column:role"`
	IsActive           bool        `gorm:"column:is_active"`
	MustChangePassword bool        `gorm:"column:must_change_password"`
	CreatedAt          time.Time   `gorm:"column:created_at"`
	UpdatedAt          time.Time   `gorm:"column:updated_at"`
	DeletedAt          *time.Time  `gorm:"column:deleted_at"`
}

func (userRecord) TableName() string { return "users" }

func (record userRecord) domainUser() domain.User {
	return domain.User{
		ID:                 record.ID,
		FullName:           record.FullName,
		Username:           record.Username,
		Role:               record.Role,
		IsActive:           record.IsActive,
		MustChangePassword: record.MustChangePassword,
		CreatedAt:          record.CreatedAt,
		UpdatedAt:          record.UpdatedAt,
		DeletedAt:          record.DeletedAt,
	}
}

type packageProjection struct {
	ID              uuid.UUID  `gorm:"column:id"`
	Code            string     `gorm:"column:code"`
	CurrentRevision int        `gorm:"column:current_revision"`
	Name            string     `gorm:"column:name"`
	Description     string     `gorm:"column:description"`
	UnitPrice       int64      `gorm:"column:unit_price"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
	DeletedAt       *time.Time `gorm:"column:deleted_at"`
}

func (record packageProjection) domainPackage() domain.Package {
	return domain.Package{
		ID:              record.ID,
		Code:            record.Code,
		CurrentRevision: record.CurrentRevision,
		Name:            record.Name,
		Description:     record.Description,
		UnitPrice:       record.UnitPrice,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
		DeletedAt:       record.DeletedAt,
	}
}

type terminalRecord struct {
	ID             uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	InstallationID string     `gorm:"column:installation_id"`
	Name           string     `gorm:"column:name"`
	PublicKey      []byte     `gorm:"column:public_key"`
	Platform       string     `gorm:"column:platform"`
	DeviceModel    *string    `gorm:"column:device_model"`
	OSVersion      *string    `gorm:"column:os_version"`
	AppVersion     *string    `gorm:"column:app_version"`
	IsActive       bool       `gorm:"column:is_active"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	RevokedAt      *time.Time `gorm:"column:revoked_at"`
}

func (terminalRecord) TableName() string { return "terminals" }

func (record terminalRecord) domainTerminal() domain.Terminal {
	return domain.Terminal{
		ID:             record.ID,
		InstallationID: record.InstallationID,
		Name:           record.Name,
		PublicKey:      record.PublicKey,
		Algorithm:      "Ed25519",
		Platform:       record.Platform,
		DeviceModel:    record.DeviceModel,
		OSVersion:      record.OSVersion,
		AppVersion:     record.AppVersion,
		IsActive:       record.IsActive,
		CreatedAt:      record.CreatedAt,
		RevokedAt:      record.RevokedAt,
	}
}
