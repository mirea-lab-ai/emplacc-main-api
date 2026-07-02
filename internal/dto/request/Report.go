package request

import (
	"time"

	"github.com/google/uuid"
)

type TomorrowPlanCreateRequest struct {
	Description string `json:"description"`
	TaskId      string `json:"task_id"`
}

type ReportsByDateInXLSX struct {
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
}

type CompletedWorkCreateRequest struct {
	Description string `json:"description"`
	TaskID      string `json:"task_id"`
}

type HelpRequest struct {
	HelperID    string  `json:"helper_id"`
	Description string  `json:"description"`
	Status      *string `json:"status"`
}

type ReportCreateRequest struct {
	UserId       string                       `json:"user_id"`
	ReportDate   *time.Time                   `json:"report_date"`
	CompleteWork []CompletedWorkCreateRequest `json:"complete_work"`
	PlanTomorrow []TomorrowPlanCreateRequest  `json:"plan_tomorrow"`
	Problems     []string                     `json:"problems,omitempty"`
	Helps        []HelpRequest                `json:"help"`
}

type ReportUpdateRequest struct {
	UserId     *string    `json:"user_id"`
	ReportDate *time.Time `json:"report_date"`
	Checked    *int8      `json:"checked"`
}

type HelpRequestUpdateRequest struct {
	HelperID    *string `json:"helper_id"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

type CompletedWorkUpdateRequest struct {
	Description *string `json:"description"`
	TaskId      string  `json:"task_id"`
}

type TomorrowPlansUpdateRequest struct {
	Description *string `json:"description"`
}

type ReportListRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

type ReportsByProjectId struct {
	ProjectID string `json:"project_id"`
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
}

type Problem struct {
	ID          string    `json:"id"`
	Description []string  `json:"description"`
	CreatorId   uuid.UUID `json:"creator_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Name        string    `json:"name"`
}

type CompletedWorkReplaceRequest struct {
	ID          *string `json:"id,omitempty"`
	Description string  `json:"description"`
	TaskID      *string `json:"task_id,omitempty"`
}

type TomorrowPlanReplaceRequest struct {
	ID          *string `json:"id,omitempty"`
	Description string  `json:"description"`
}

type HelpRequestReplaceRequest struct {
	ID          *string `json:"id,omitempty"`
	HelperID    *string `json:"helper_id,omitempty"`
	Description string  `json:"description"`
	Status      *string `json:"status,omitempty"`
}

type ReportReplaceRequest struct {
	UserId     string     `json:"user_id"`
	ReportDate *time.Time `json:"report_date,omitempty"`
	Checked    *int8      `json:"checked,omitempty"`

	CompleteWork []CompletedWorkReplaceRequest `json:"complete_work,omitempty"`
	PlanTomorrow []TomorrowPlanReplaceRequest  `json:"plan_tomorrow,omitempty"`
	Problems     []string                      `json:"problems,omitempty"`
	Helps        []HelpRequestReplaceRequest   `json:"help,omitempty"`
}
