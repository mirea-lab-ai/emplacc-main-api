package response

import (
	"time"
)

type AttendanceUniversalResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type AttendanceResponse struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Date          time.Time `json:"date"`
	WorkdayHours  int16     `json:"workday_hours"`
	PlannedStart  time.Time `json:"planned_start"`
	ActualStart   time.Time `json:"actual_start"`
	Status        string    `json:"status"`
	Commits       int16     `json:"commits"`
	MergeRequests int16     `json:"merge_requests"`
	CodeReviews   int16     `json:"code_reviews"`
	EndWork       time.Time `json:"end_work"`
	Deleted       bool      `json:"deleted"`
	UpdatedAt     time.Time `json:"updated_at"`
	CreatedAt     time.Time `json:"created_at"`
}

type AttendancesByUserId struct {
	UserID      string               `json:"user_id"`
	Attendances []AttendanceResponse `json:"attendances"`
}

type AttendancesListResponse struct {
	Attendances []AttendanceResponse `json:"attendances"`
	TotalCount  int64                `json:"total_count"`
	Page        int                  `json:"page"`
	PageSize    int                  `json:"page_size"`
}
