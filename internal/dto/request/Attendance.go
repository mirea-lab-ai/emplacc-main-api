package request

import (
	"time"
)

type AttendanceCreateRequest struct {
	UserId        string     `json:"user_id"`
	Date          *time.Time `json:"date"`
	WorkdayHours  *int16     `json:"workday_hours"`
	PlannedStart  *time.Time `json:"planned_start"`
	ActualStart   *time.Time `json:"actual_start"`
	Status        *string    `json:"status"`
	Commits       *int16     `json:"commits"`
	MergeRequests *int16     `json:"merge_requests"`
	CodeReviews   *int16     `json:"code_reviews"`
	EndWork       *time.Time `json:"end_work"`
}

type AttendanceUpdateRequest struct {
	Date          *time.Time `json:"date"`
	WorkdayHours  *int16     `json:"workday_hours"`
	PlannedStart  *time.Time `json:"planned_start"`
	ActualStart   *time.Time `json:"actual_start"`
	Status        *string    `json:"status"`
	Commits       *int16     `json:"commits"`
	MergeRequests *int16     `json:"merge_requests"`
	CodeReviews   *int16     `json:"code_reviews"`
	EndWork       *time.Time `json:"end_work"`
}

type AttendanceByUserIdRequest struct {
	UserID   string `json:"user_id"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type AttendanceListRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}
