package models

import (
	"time"

	"github.com/google/uuid"
)

func (User) TableName() string     { return "users" }
func (Role) TableName() string     { return "roles" }
func (UserRole) TableName() string { return "user_roles" }

type User struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email         string    `gorm:"size:100"`
	IsActive      bool
	CreatedAt     time.Time `gorm:"type:timestamp"`
	TgID          string    `gorm:"size:50"`
	TgUserID      int64
	Profession    string `gorm:"size:50"`
	EmailVerified bool
	FirstName     string `gorm:"size:50"`
	LastName      string `gorm:"size:50"`
	LastLogin     time.Time
	AvatarURL     string    `gorm:"size:500"`
	Deleted       bool      `gorm:"type:boolean"`
	UpdatedAt     time.Time `gorm:"type:timestamp"`

	// Преференция email-уведомлений. nil (старые записи) трактуется как «включено».
	EmailNotifications *bool `gorm:"default:true"`

	// вместо many2many — явная джойн-модель
	UserRoles []UserRole `gorm:"foreignKey:UserID;references:ID"`
}

type Role struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Name        *string    `gorm:"size:50"`
	Description *string    `gorm:"size:200"`
	Deleted     *bool      `gorm:"type:boolean"`
	CreatedAt   *time.Time `gorm:"type:timestamp"`
	UpdatedAt   *time.Time `gorm:"type:timestamp"`

	// вместо many2many — явная джойн-модель
	UserRoles []UserRole `gorm:"foreignKey:RoleID;references:ID"`
}

type UserRole struct {
	RoleID     uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID     uuid.UUID `gorm:"type:uuid;primaryKey"`
	AssignedAt *time.Time
	AssignedBy *uuid.UUID `gorm:"type:uuid"`
	Deleted    *bool      `gorm:"type:boolean"`
	CreatedAt  *time.Time `gorm:"type:timestamp"`
	UpdatedAt  *time.Time `gorm:"type:timestamp"`

	//Role *Role `gorm:"foreignKey:RoleID;references:ID;constraint:OnDelete:CASCADE"`
	//User *User `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
}
