package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/ports"
	"emplacc-api/internal/utils"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ReportService interface {
	GetAllReports(page, pageSize int) ([]models.DailyReport, int64, error)
	GetAllReportsByUserId(userID uuid.UUID, page, pageSize int) ([]models.DailyReport, int64, error)
	GetReport(reportID uuid.UUID) (*models.DailyReport, error)
	GetReportsByTaskId(taskID uuid.UUID) ([]models.DailyReport, error)
	GetReportsByProjectId(projectID uuid.UUID) ([]models.DailyReport, error)
	CreateReport(req request.ReportCreateRequest) (uuid.UUID, error)
	UpdateReport(reportID uuid.UUID, req request.ReportReplaceRequest) error
	DeleteReport(reportID uuid.UUID) error
	UpdateHelpRequest(helpID uuid.UUID, req request.HelpRequestUpdateRequest) error
	UpdateCompletedWork(cwID uuid.UUID, req request.CompletedWorkUpdateRequest) error
	UpdateTomorrowPlans(tpID uuid.UUID, req request.TomorrowPlansUpdateRequest) error
	GetHelpRequestsForUser(userID uuid.UUID) ([]models.HelpRequest, error)
	DeleteHelpRequest(requestID uuid.UUID) error
	GetReportByDateInXLSX(req request.ReportsByDateInXLSX) (*response.XLSXReportData, error)
	GetTomorrowPlansForXLSX() (*response.TomorrowPlansXLSXData, error)
}

type reportService struct {
	repo ports.ReportRepository
}

func NewReportService(repo ports.ReportRepository) ReportService {
	return &reportService{
		repo: repo,
	}
}

func (s *reportService) GetAllReports(page, pageSize int) ([]models.DailyReport, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllReports(pageSize, offset)
}

func (s *reportService) GetReportByDateInXLSX(req request.ReportsByDateInXLSX) (*response.XLSXReportData, error) {
	reports, err := s.repo.GetReportByDateInXLSX(req.StartDate, req.EndDate)
	if err != nil {
		return nil, err
	}

	// 1. Все даты в диапазоне
	var dates []time.Time
	current := req.StartDate
	for !current.After(req.EndDate) {
		dates = append(dates, current.Truncate(24*time.Hour))
		current = current.AddDate(0, 0, 1)
	}

	// 2. Группировка
	userMap := make(map[string]map[time.Time][]string)
	userOrder := []string{}

	for _, report := range reports {
		if report.User == nil || report.ReportDate == nil {
			continue
		}

		firstName := report.User.FirstName
		lastName := report.User.LastName
		userName := strings.TrimSpace(firstName + " " + lastName)
		if userName == "" {
			userName = report.User.Email
		}
		if userName == "" {
			userName = report.User.ID.String()
		}

		if _, exists := userMap[userName]; !exists {
			userMap[userName] = make(map[time.Time][]string)
			userOrder = append(userOrder, userName)
		}

		reportDate := report.ReportDate.Truncate(24 * time.Hour)

		for _, work := range report.CompletedWork {
			isDeleted := work.Deleted != nil && *work.Deleted
			if isDeleted {
				continue
			}
			if work.Description == nil || *work.Description == "" {
				continue
			}

			// === Получаем название проекта ===
			projectName := "Без проекта"
			if work.Task != nil &&
				work.Task.Status != nil &&
				work.Task.Status.Board != nil &&
				work.Task.Status.Board.Project != nil {
				if work.Task.Status.Board.Project.Name != nil && *work.Task.Status.Board.Project.Name != "" {
					projectName = *work.Task.Status.Board.Project.Name
				}
			}

			// === Получаем название задачи ===
			taskName := "Без названия"
			if work.Task != nil && work.Task.Name != nil && *work.Task.Name != "" {
				taskName = *work.Task.Name
			}

			// === Формируем итоговую строку ===
			workText := projectName + ": " + taskName + ": " + *work.Description
			userMap[userName][reportDate] = append(userMap[userName][reportDate], workText)
		}
	}

	return &response.XLSXReportData{
		Users: userOrder,
		Dates: dates,
		Grid:  userMap,
	}, nil
}

func (s *reportService) GetAllReportsByUserId(userID uuid.UUID, page, pageSize int) ([]models.DailyReport, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllReportsByUserId(userID, pageSize, offset)
}

func (s *reportService) GetReport(reportID uuid.UUID) (*models.DailyReport, error) {
	return s.repo.GetReport(reportID)
}

func (s *reportService) GetReportsByTaskId(taskID uuid.UUID) ([]models.DailyReport, error) {
	return s.repo.GetReportsByTaskId(taskID)
}

func (s *reportService) GetReportsByProjectId(projectID uuid.UUID) ([]models.DailyReport, error) {
	return s.repo.GetReportsByProjectId(projectID)
}

func (s *reportService) GetTomorrowPlansForXLSX() (*response.TomorrowPlansXLSXData, error) {
	plans, err := s.repo.GetLatestTomorrowPlans()
	if err != nil {
		return nil, err
	}

	userPlansMap := make(map[uuid.UUID]*response.UserTomorrowPlans)

	for _, plan := range plans {
		var userID uuid.UUID
		var reportDate time.Time
		var userName, userEmail string

		if plan.Report != nil {
			userID = plan.Report.UserID
			reportDate = utils.GetTime(plan.Report.ReportDate)

			if plan.Report.User != nil {
				firstName := plan.Report.User.FirstName
				lastName := plan.Report.User.LastName
				if firstName != "" || lastName != "" {
					userName = strings.TrimSpace(firstName + " " + lastName)
				} else {
					userName = plan.Report.User.Email
				}
				userEmail = plan.Report.User.Email
			}
		} else {
			continue
		}

		if _, exists := userPlansMap[userID]; !exists {
			userPlansMap[userID] = &response.UserTomorrowPlans{
				UserID:     userID.String(),
				UserName:   userName,
				UserEmail:  userEmail,
				ReportDate: reportDate,
				Plans:      []response.TomorrowPlan{},
			}
		}

		taskName := "Без задачи"
		projectName := "Без проекта"

		if plan.Task != nil {
			if plan.Task.Name != nil && *plan.Task.Name != "" {
				taskName = *plan.Task.Name
			}

			if plan.Task.Status != nil &&
				plan.Task.Status.Board != nil &&
				plan.Task.Status.Board.Project != nil {
				if plan.Task.Status.Board.Project.Name != nil && *plan.Task.Status.Board.Project.Name != "" {
					projectName = *plan.Task.Status.Board.Project.Name
				}
			}
		}

		description := ""
		if plan.Description != nil {
			description = *plan.Description
		}

		tomorrowPlan := response.TomorrowPlan{
			ID:          plan.ID.String(),
			Description: description,
			TaskName:    taskName,
			ProjectName: projectName,
			CreatedAt:   utils.GetTime(plan.CreatedAt),
		}

		userPlansMap[userID].Plans = append(userPlansMap[userID].Plans, tomorrowPlan)
	}

	users := make([]response.UserTomorrowPlans, 0, len(userPlansMap))
	for _, userPlans := range userPlansMap {
		users = append(users, *userPlans)
	}

	sort.Slice(users, func(i, j int) bool {
		return users[i].UserName < users[j].UserName
	})

	return &response.TomorrowPlansXLSXData{
		Users: users,
	}, nil
}

func (s *reportService) CreateReport(req request.ReportCreateRequest) (uuid.UUID, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return uuid.Nil, errors.New("invalid user id")
	}

	now := time.Now()
	delFalse := false
	zero := int8(0)
	rep := models.DailyReport{
		ID:         uuid.New(),
		UserID:     userID,
		ReportDate: req.ReportDate,
		CreatedAt:  &now,
		Deleted:    &delFalse,
		Checked:    &zero,
	}

	err = s.repo.CreateReportWithRelations(rep, req)
	if err != nil {
		return uuid.Nil, err
	}

	return rep.ID, nil
}

func (s *reportService) UpdateReport(reportID uuid.UUID, req request.ReportReplaceRequest) error {
	report, err := s.repo.GetReport(reportID)
	if err != nil {
		if err.Error() == "report not found" {
			return errors.New("report not found")
		}
		return err
	}

	now := time.Now()
	if err := s.repo.Transaction(func(tx ports.ReportRepository) error {
		updateData := map[string]interface{}{"updated_at": &now}
		if req.UserId != "" {
			if uid, err := uuid.Parse(req.UserId); err == nil {
				updateData["user_id"] = uid
			} else {
				return errors.New("invalid user_id")
			}
		}
		if req.ReportDate != nil {
			updateData["report_date"] = req.ReportDate
		}
		if req.Checked != nil {
			updateData["checked"] = req.Checked
		}
		// Передаем разыменованный report
		if err := tx.UpdateReport(*report, updateData); err != nil {
			return err
		}

		// Обновляем связанные сущности
		return tx.UpdateReportRelations(reportID, req, now)
	}); err != nil {
		return err
	}

	return nil
}

func (s *reportService) DeleteReport(reportID uuid.UUID) error {
	deleted, err := s.repo.DeleteReport(reportID)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("report not found")
	}

	return nil
}

func (s *reportService) UpdateHelpRequest(helpID uuid.UUID, req request.HelpRequestUpdateRequest) error {
	updateData := make(map[string]interface{})
	if req.Description != nil {
		updateData["description"] = *req.Description
	}
	if req.HelperID != nil {
		updateData["helper_id"] = *req.HelperID
	}
	if req.Status != nil {
		updateData["status"] = *req.Status
	}

	if len(updateData) == 0 {
		return errors.New("no fields to update")
	}

	now := time.Now()
	updateData["updated_at"] = &now

	updated, err := s.repo.UpdateHelpRequest(helpID, updateData)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("help request not found")
	}

	return nil
}

func (s *reportService) UpdateCompletedWork(cwID uuid.UUID, req request.CompletedWorkUpdateRequest) error {
	updateData := make(map[string]interface{})
	if req.Description != nil {
		updateData["description"] = *req.Description
	}

	if len(updateData) == 0 {
		return errors.New("no fields to update")
	}

	now := time.Now()
	updateData["updated_at"] = &now

	updated, err := s.repo.UpdateCompletedWork(cwID, updateData)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("completed work not found")
	}

	return nil
}

func (s *reportService) UpdateTomorrowPlans(tpID uuid.UUID, req request.TomorrowPlansUpdateRequest) error {
	updateData := make(map[string]interface{})
	if req.Description != nil {
		updateData["description"] = *req.Description
	}

	if len(updateData) == 0 {
		return errors.New("no fields to update")
	}

	now := time.Now()
	updateData["updated_at"] = &now

	updated, err := s.repo.UpdateTomorrowPlans(tpID, updateData)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("tomorrow plans not found")
	}

	return nil
}

func (s *reportService) GetHelpRequestsForUser(userID uuid.UUID) ([]models.HelpRequest, error) {
	return s.repo.GetHelpRequestsForUser(userID)
}

func (s *reportService) DeleteHelpRequest(requestID uuid.UUID) error {
	deleted, err := s.repo.DeleteHelpRequest(requestID)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("help request not found")
	}

	return nil
}
