package ports

import (
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"

	"github.com/google/uuid"
)

type ReportRepository interface {
	GetAllReports(limit, offset int) ([]models.DailyReport, int64, error)
	GetAllReportsByUserId(userID uuid.UUID, limit, offset int) ([]models.DailyReport, int64, error)
	GetReport(reportID uuid.UUID) (*models.DailyReport, error)
	GetReportsByTaskId(taskID uuid.UUID) ([]models.DailyReport, error)
	GetReportsByProjectId(projectID uuid.UUID) ([]models.DailyReport, error)
	CreateReportWithRelations(rep models.DailyReport, req request.ReportCreateRequest) error
	UpdateReport(report models.DailyReport, updateData map[string]interface{}) error
	UpdateReportRelations(reportID uuid.UUID, req request.ReportReplaceRequest, now time.Time) error
	DeleteReport(reportID uuid.UUID) (bool, error)
	UpdateHelpRequest(helpID uuid.UUID, updateData map[string]interface{}) (bool, error)
	UpdateCompletedWork(cwID uuid.UUID, updateData map[string]interface{}) (bool, error)
	UpdateTomorrowPlans(tpID uuid.UUID, updateData map[string]interface{}) (bool, error)
	GetHelpRequestsForUser(userID uuid.UUID) ([]models.HelpRequest, error)
	DeleteHelpRequest(requestID uuid.UUID) (bool, error)
	Transaction(txFunc func(ReportRepository) error) error
	GetReportByDateInXLSX(startDate, endDate time.Time) ([]models.DailyReport, error)
	GetLatestTomorrowPlans() ([]models.TomorrowPlans, error)
}
