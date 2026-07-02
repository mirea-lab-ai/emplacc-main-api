package models

import (
	"time"

	"github.com/google/uuid"
)

func (DailyReport) TableName() string   { return "daily_reports" }
func (ReportProblem) TableName() string { return "report_problems" }
func (HelpRequest) TableName() string   { return "help_requests" }
func (CompletedWork) TableName() string { return "completed_works" }
func (TomorrowPlans) TableName() string { return "tomorrow_plans" }

type DailyReport struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID     uuid.UUID  `gorm:"type:uuid;index"`
	Checked    *int8      `gorm:"type:int8"`
	ReportDate *time.Time `gorm:"type:date"`
	CreatedAt  *time.Time `gorm:"type:date"`
	UpdatedAt  *time.Time `gorm:"type:date"`
	Deleted    *bool      `gorm:"type:boolean"`

	User           *User           `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	HelpRequests   []HelpRequest   `gorm:"foreignKey:ReportID;references:ID"`
	CompletedWork  []CompletedWork `gorm:"foreignKey:ReportID;references:ID"`
	TomorrowPlans  []TomorrowPlans `gorm:"foreignKey:ReportID;references:ID"`
	ReportProblems []ReportProblem `gorm:"foreignKey:ReportID;references:ID"`
}

type ReportProblem struct {
	ReportID  uuid.UUID  `gorm:"type:uuid;primaryKey;column:report_id"`
	ProblemID uuid.UUID  `gorm:"type:uuid;primaryKey;column:problem_id"`
	CreatedAt *time.Time `gorm:"type:timestamp"`
	UpdatedAt *time.Time `gorm:"type:timestamp"`
	Deleted   *bool      `gorm:"type:boolean;default:false"`

	Report  *DailyReport `gorm:"foreignKey:ReportID;references:ID;constraint:OnDelete:CASCADE"`
	Problem *Problem     `gorm:"foreignKey:ProblemID;references:ID;constraint:OnDelete:CASCADE"`
}

type HelpRequest struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey"`
	HelperID    *uuid.UUID `gorm:"type:uuid"`
	Description *string    `gorm:"type:varchar(255)"`
	Deleted     *bool      `gorm:"type:boolean;default:false"`
	ReportID    *uuid.UUID `gorm:"type:uuid"`
	Status      *string    `gorm:"type:varchar(30)"`
	UpdatedAt   *time.Time `gorm:"type:timestamp"`
	CreatedAt   *time.Time `gorm:"type:timestamp"`

	Helper *User        `gorm:"foreignKey:HelperID;references:ID"`
	Report *DailyReport `gorm:"foreignKey:ReportID;references:ID"`
}

type CompletedWork struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Description *string    `gorm:"type:text"`
	Deleted     *bool      `gorm:"type:boolean;default:false"`
	ReportID    *uuid.UUID `gorm:"type:uuid"`
	TaskID      *uuid.UUID `gorm:"type:uuid"`
	UpdatedAt   *time.Time `gorm:"type:timestamp"`
	CreatedAt   *time.Time `gorm:"type:timestamp"`

	Report *DailyReport `gorm:"foreignKey:ReportID;references:ID"`
	Task   *Task        `gorm:"foreignKey:TaskID;references:ID"`
}

type TomorrowPlans struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TaskID      *uuid.UUID `gorm:"type:uuid"`
	Description *string    `gorm:"type:text"`
	Deleted     *bool      `gorm:"type:boolean;default:false"`
	ReportID    *uuid.UUID `gorm:"type:uuid"`
	UpdatedAt   *time.Time `gorm:"type:timestamp"`
	CreatedAt   *time.Time `gorm:"type:timestamp"`

	Report *DailyReport `gorm:"foreignKey:ReportID;references:ID"`
	Task   *Task        `gorm:"foreignKey:TaskID;references:ID"`
}
