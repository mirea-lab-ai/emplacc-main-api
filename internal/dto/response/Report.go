package response

import (
	"time"
)

type ReportUniversalResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type ReportResponse struct {
	ID            string            `json:"id"`
	UserID        string            `json:"user_id"`
	ReportDate    time.Time         `json:"report_date"`
	CompletedWork []CompletedWork   `json:"completed_work"`
	PlanTomorrow  []TomorrowPlans   `json:"plan_tomorrow"`
	HelpRequest   []HelpRequestItem `json:"help_requests,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	UserInfo      UserShort         `json:"user_info"`
	Checked       int8              `json:"checked"`
	Problems      []ProblemResponse `json:"problem"`
}

type ReportFullResponse struct {
	ID            string                  `json:"id"`
	UserID        string                  `json:"user_id"`
	ReportDate    time.Time               `json:"report_date"`
	CompletedWork []CompletedWorkWithTask `json:"completed_work"`
	PlanTomorrow  []TomorrowPlansWithTask `json:"plan_tomorrow"`
	HelpRequest   []HelpRequestItem       `json:"help_requests,omitempty"`
	CreatedAt     time.Time               `json:"created_at"`
	UpdatedAt     time.Time               `json:"updated_at"`
	UserInfo      UserShort               `json:"user_info"`
	Checked       int8                    `json:"checked"`
	Problems      []ProblemResponse       `json:"problem"`
}

type TomorrowPlansWithTask struct {
	ID          string        `json:"id"`
	Description string        `json:"description"`
	Task        TaskForReport `json:"task"`
}

type CompletedWorkWithTask struct {
	ID          string        `json:"id"`
	Description string        `json:"description"`
	Task        TaskForReport `json:"task"`
}

type InTaskBoard struct {
	BoardName string `json:"name"`
	BoardId   string `json:"id"`
}

type InTaskProject struct {
	ProjectName string `json:"name"`
	ProjectId   string `json:"id"`
}

type TaskForReport struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Board       InTaskBoard   `json:"board"`
	Project     InTaskProject `json:"project"`
}

type ProblemResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description []string  `json:"description"`
	CreatorId   string    `json:"creator_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TomorrowPlans struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	TaskId      string `json:"task_id"`
}

type CompletedWork struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	TaskId      string `json:"task_id"`
}

type HelpRequestItem struct {
	ID          string `json:"id"`
	HelperID    string `json:"helper_id"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

type ReportListResponse struct {
	Reports    []ReportResponse `json:"reports"`
	TotalCount int64            `json:"total_count"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
}

type ReportListByTaskId struct {
	TaskID  string           `json:"task_id"`
	Reports []ReportResponse `json:"reports"`
}

type ReportListByProjectId struct {
	ProjectID string           `json:"project_id"`
	Reports   []ReportResponse `json:"reports"`
}

type HelpRequestWithAssignerID struct {
	HelpRequest   HelpRequestItem `json:"help_request"`
	UserFirstName string          `json:"user_first_name"`
	UserLastName  string          `json:"user_last_name"`
}

type HelpRequestsForUser struct {
	HelpRequests []HelpRequestWithAssignerID `json:"help_requests"`
}

type XLSXReportEntry struct {
	UserName string
	Date     time.Time
	Works    []string
}

type XLSXReportData struct {
	Users []string                          // порядок пользователей
	Dates []time.Time                       // все даты в диапазоне
	Grid  map[string]map[time.Time][]string // user → date → работы
}

type TomorrowPlansXLSXData struct {
	Users []UserTomorrowPlans `json:"users"`
}

// Планы пользователя
type UserTomorrowPlans struct {
	UserID     string         `json:"user_id"`
	UserName   string         `json:"user_name"`
	UserEmail  string         `json:"user_email"`
	ReportDate time.Time      `json:"report_date"`
	Plans      []TomorrowPlan `json:"plans"`
}

// Детали плана
type TomorrowPlan struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	TaskName    string    `json:"task_name"`
	ProjectName string    `json:"project_name"`
	CreatedAt   time.Time `json:"created_at"`
}
