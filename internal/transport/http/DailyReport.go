package httpapi

import (
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/service"
	"emplacc-api/internal/utils"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"
)

type ReportController struct {
	reportService  service.ReportService
	freshAvatarURL func(string) string
}

func NewReportController(reportService service.ReportService, freshAvatarURL func(string) string) *ReportController {
	return &ReportController{
		reportService:  reportService,
		freshAvatarURL: freshAvatarURL,
	}
}

func RegisterReportRoutes(e Router, reportService service.ReportService, freshAvatarURL func(string) string, employeeMw echo.MiddlewareFunc, managerMw echo.MiddlewareFunc) {
	controller := NewReportController(reportService, freshAvatarURL)
	g := e.Group("/report")
	// Чтение — все авторизованные
	g.GET("/all/:page/:pagesize", controller.GetAllReports)
	g.GET("/:id", controller.GetReport)
	g.GET("/task/:id", controller.GetReportsByTaskId)
	g.GET("/project/:id", controller.GetReportsByProjectId)
	g.GET("/user/:id/:page/:pagesize", controller.GetAllReportsByUserId)
	g.GET("/help-requests-by-user-id/:id", controller.GetHelpRequestsForUser)
	g.GET("/export/tomorrow-plans/xlsx", controller.ExportTomorrowPlansToXLSX)
	// Запись — employee и выше
	g.POST("", controller.CreateReport, employeeMw)
	g.PATCH("/:id", controller.UpdateReport, employeeMw)
	g.PATCH("/help-request/:id", controller.UpdateHelpRequest, employeeMw)
	g.PATCH("/completed-work/:id", controller.UpdateCompletedWork, employeeMw)
	g.PATCH("/tomorrow-plans/:id", controller.UpdateTomorrowPlans, employeeMw)
	g.POST("/export/xlsx", controller.GetReportByDateInXLSX, employeeMw)
	// Удаление — manager и admin
	g.DELETE("/:id", controller.DeleteReport, managerMw)
	g.DELETE("/help-request/:id", controller.DeleteHelpRequest, managerMw)
}

// GetAllReports godoc
// @Summary Получение списка всех отчетов
// @Description Получает список всех отчетов с учетом пагинации, исключая удаленные
// @Tags Reports
// @Accept json
// @Produce json
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ReportListResponse "Список отчетов успешно получен"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 404 {object} map[string]string "Пользователь, запрос на помощь, выполненная работа или план на завтра не найдены"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении отчетов"
// @Router /report/all/{page}/{pagesize} [get]
func (rc *ReportController) GetAllReports(c echo.Context) error {
	page, _ := strconv.Atoi(c.Param("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Param("pagesize"))
	if pageSize <= 0 {
		pageSize = 10
	}

	reports, totalCount, err := rc.reportService.GetAllReports(page, pageSize)
	if err != nil {
		log.Printf("service error (get all reports): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подсчете отчетов"})
	}

	out := response.ReportListResponse{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
	}

	for _, r := range reports {
		userInfo := response.UserShort{
			ID:        r.UserID.String(),
			FirstName: r.User.FirstName,
			LastName:  r.User.LastName,
			AvatarURL: rc.freshAvatarURL(r.User.AvatarURL),
		}
		var complWork []response.CompletedWork
		for _, w := range r.CompletedWork {
			complWork = append(complWork, response.CompletedWork{
				ID:          w.ID.String(),
				Description: utils.GetString(w.Description),
				TaskId:      utils.GetUUIDString(w.TaskID),
			})
		}
		var plans []response.TomorrowPlans
		for _, p := range r.TomorrowPlans {
			plans = append(plans, response.TomorrowPlans{
				ID:          p.ID.String(),
				Description: utils.GetString(p.Description),
				TaskId:      utils.GetUUIDString(p.TaskID),
			})
		}
		var problemsResp []response.ProblemResponse
		for _, rp := range r.ReportProblems {
			if rp.Problem != nil {
				problemsResp = append(problemsResp, response.ProblemResponse{
					ID:          rp.Problem.ID.String(),
					Name:        utils.GetString(rp.Problem.Name),
					Description: rp.Problem.Description,
					CreatorId:   utils.GetUUIDString(rp.Problem.CreatorID),
					CreatedAt:   utils.GetTime(rp.Problem.CreatedAt),
					UpdatedAt:   utils.GetTime(rp.Problem.UpdatedAt),
				})
			}
		}
		helpResp := []response.HelpRequestItem{}
		for _, hr := range r.HelpRequests {
			helpResp = append(helpResp, response.HelpRequestItem{
				ID:          hr.ID.String(),
				HelperID:    utils.GetUUIDString(hr.HelperID),
				Description: utils.GetString(hr.Description),
				Status:      utils.GetString(hr.Status),
			})
		}
		out.Reports = append(out.Reports, response.ReportResponse{
			ID:            r.ID.String(),
			UserID:        r.UserID.String(),
			ReportDate:    utils.GetTime(r.ReportDate),
			CompletedWork: complWork,
			PlanTomorrow:  plans,
			HelpRequest:   helpResp,
			CreatedAt:     utils.GetTime(r.CreatedAt),
			UpdatedAt:     utils.GetTime(r.UpdatedAt),
			UserInfo:      userInfo,
			Problems:      problemsResp,
			Checked:       utils.GetInt8(r.Checked),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// ExportTomorrowPlansToXLSX godoc
// @Summary Экспорт планов на завтра в XLSX
// @Description Генерирует XLSX-файл с планами на завтра из последних отчетов всех пользователей
// @Tags Reports
// @Accept json
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security BearerAuth
// @Success 200 {string} file "XLSX-файл с планами на завтра"
// @Failure 500 {object} map[string]string "Ошибка при генерации отчёта"
// @Router /report/export/tomorrow-plans/xlsx [get]
func (rc *ReportController) ExportTomorrowPlansToXLSX(c echo.Context) error {
	data, err := rc.reportService.GetTomorrowPlansForXLSX()
	if err != nil {
		log.Printf("service error (get tomorrow plans for XLSX): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подготовке данных"})
	}

	f := excelize.NewFile()

	summarySheet := "Планы на завтра"
	f.NewSheet(summarySheet)

	f.SetCellValue(summarySheet, "A1", "Отчет по планам на завтра")
	f.SetCellValue(summarySheet, "A2", "Дата формирования: "+time.Now().Format("02.01.2006 15:04"))
	f.SetCellValue(summarySheet, "A3", "Всего пользователей: "+strconv.Itoa(len(data.Users)))
	f.SetCellValue(summarySheet, "A4", "Всего планов: "+strconv.Itoa(calculateTotalPlans(data.Users)))

	f.SetCellValue(summarySheet, "A5", "")

	summaryHeaders := []interface{}{
		"Пользователь", "Дата отчета", "Задача", "Проект", "Описание плана",
		"Создано",
	}

	if err := f.SetSheetRow(summarySheet, "A6", &summaryHeaders); err != nil {
		log.Printf("XLSX header error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при создании XLSX"})
	}

	rowIndex := 7
	totalPlans := 0

	for _, user := range data.Users {
		userSheet := user.UserName
		if userSheet == "" {
			userSheet = user.UserEmail
		}

		f.NewSheet(userSheet)

		f.SetCellValue(userSheet, "A1", "Пользователь: "+user.UserName)
		if user.UserEmail != "" {
			f.SetCellValue(userSheet, "A2", "Email: "+user.UserEmail)
		}
		f.SetCellValue(userSheet, "A3", "Дата отчета: "+user.ReportDate.Format("02.01.2006"))
		f.SetCellValue(userSheet, "A4", "Количество планов: "+strconv.Itoa(len(user.Plans)))

		f.SetCellValue(userSheet, "A5", "")

		userHeaders := []interface{}{
			"Дата отчета", "Задача", "Проект", "Описание плана", "Создано",
		}
		f.SetSheetRow(userSheet, "A6", &userHeaders)

		userRowIndex := 7

		for _, plan := range user.Plans {
			totalPlans++

			summaryRow := []interface{}{
				user.UserName,
				user.ReportDate.Format("02.01.2006"),
				plan.TaskName,
				plan.ProjectName,
				plan.Description,
				plan.CreatedAt.Format("02.01.2006 15:04"),
			}

			axis := fmt.Sprintf("A%d", rowIndex)
			if err := f.SetSheetRow(summarySheet, axis, &summaryRow); err != nil {
				log.Printf("XLSX summary row error: %v", err)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при заполнении XLSX"})
			}
			rowIndex++

			userRow := []interface{}{
				user.ReportDate.Format("02.01.2006"),
				plan.TaskName,
				plan.ProjectName,
				plan.Description,
				plan.CreatedAt.Format("02.01.2006 15:04"),
			}

			userAxis := fmt.Sprintf("A%d", userRowIndex)
			if err := f.SetSheetRow(userSheet, userAxis, &userRow); err != nil {
				log.Printf("XLSX user row error: %v", err)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при заполнении XLSX"})
			}
			userRowIndex++
		}

		f.SetColWidth(userSheet, "A", "E", 20)
		f.SetColWidth(userSheet, "B", "C", 25)
		f.SetColWidth(userSheet, "D", "D", 40)

		headerStyle, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true, Size: 12},
		})
		f.SetCellStyle(userSheet, "A1", "A4", headerStyle)

		tableStyle, _ := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
		})
		f.SetCellStyle(userSheet, "A6", fmt.Sprintf("E%d", userRowIndex), tableStyle)

		tableHeaderStyle, _ := f.NewStyle(&excelize.Style{
			Font:      &excelize.Font{Bold: true},
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "center", Horizontal: "center"},
			Fill:      excelize.Fill{Type: "pattern", Color: []string{"E6E6FA"}, Pattern: 1},
		})
		f.SetCellStyle(userSheet, "A6", "E6", tableHeaderStyle)
	}

	f.SetColWidth(summarySheet, "A", "F", 20)
	f.SetColWidth(summarySheet, "C", "D", 25)
	f.SetColWidth(summarySheet, "E", "E", 40)

	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14},
	})
	f.SetCellStyle(summarySheet, "A1", "A1", titleStyle)

	infoStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	f.SetCellStyle(summarySheet, "A2", "A4", infoStyle)

	tableStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
	})
	f.SetCellStyle(summarySheet, "A6", fmt.Sprintf("F%d", rowIndex), tableStyle)

	tableHeaderStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "center", Horizontal: "center"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"E6E6FA"}, Pattern: 1},
	})
	f.SetCellStyle(summarySheet, "A6", "F6", tableHeaderStyle)

	f.DeleteSheet("Sheet1")

	index, err := f.GetSheetIndex(summarySheet)
	if err != nil {
		log.Printf("Creating list error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при генерации файла"})
	}
	f.SetActiveSheet(index)

	buf, err := f.WriteToBuffer()
	if err != nil {
		log.Printf("XLSX buffer error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при генерации файла"})
	}

	filename := fmt.Sprintf("tomorrow_plans_%s.xlsx", time.Now().Format("2006-01-02"))

	c.Response().Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Response().Header().Set("Content-Disposition", "attachment; filename="+filename)
	return c.Blob(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

func calculateTotalPlans(users []response.UserTomorrowPlans) int {
	total := 0
	for _, user := range users {
		total += len(user.Plans)
	}
	return total
}

// GetAllReportsByUserId godoc
// @Summary Получение списка отчетов по ID пользователя
// @Description Получает список отчетов, созданных конкретным пользователем, с пагинацией и исключением удаленных записей
// @Tags Reports
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя (UUID)"
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Success 200 {object} response.ReportListResponse "Список отчетов успешно получен"
// @Failure 400 {object} map[string]string "Некорректный ID пользователя"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении отчетов"
// @Router /report/user/{id}/{page}/{pagesize} [get]
func (rc *ReportController) GetAllReportsByUserId(c echo.Context) error {
	page, _ := strconv.Atoi(c.Param("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Param("pagesize"))
	if pageSize <= 0 {
		pageSize = 10
	}
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный ID пользователя"})
	}

	reports, totalCount, err := rc.reportService.GetAllReportsByUserId(userID, page, pageSize)
	if err != nil {
		log.Printf("service error (get reports by user id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подсчете отчетов"})
	}

	out := response.ReportListResponse{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
	}

	for _, r := range reports {
		userInfo := response.UserShort{
			ID:        r.UserID.String(),
			FirstName: r.User.FirstName,
			LastName:  r.User.LastName,
			AvatarURL: rc.freshAvatarURL(r.User.AvatarURL),
		}
		var complWork []response.CompletedWork
		for _, w := range r.CompletedWork {
			complWork = append(complWork, response.CompletedWork{
				ID:          w.ID.String(),
				Description: utils.GetString(w.Description),
				TaskId:      utils.GetUUIDString(w.TaskID),
			})
		}
		var plans []response.TomorrowPlans
		for _, p := range r.TomorrowPlans {
			plans = append(plans, response.TomorrowPlans{
				ID:          p.ID.String(),
				Description: utils.GetString(p.Description),
				TaskId:      utils.GetUUIDString(p.TaskID),
			})
		}
		var problemsResp []response.ProblemResponse
		for _, rp := range r.ReportProblems {
			if rp.Problem != nil {
				problemsResp = append(problemsResp, response.ProblemResponse{
					ID:          rp.Problem.ID.String(),
					Name:        utils.GetString(rp.Problem.Name),
					Description: rp.Problem.Description,
					CreatorId:   utils.GetUUIDString(rp.Problem.CreatorID),
					CreatedAt:   utils.GetTime(rp.Problem.CreatedAt),
					UpdatedAt:   utils.GetTime(rp.Problem.UpdatedAt),
				})
			}
		}
		helpResp := []response.HelpRequestItem{}
		for _, hr := range r.HelpRequests {
			helpResp = append(helpResp, response.HelpRequestItem{
				ID:          hr.ID.String(),
				HelperID:    utils.GetUUIDString(hr.HelperID),
				Description: utils.GetString(hr.Description),
				Status:      utils.GetString(hr.Status),
			})
		}
		out.Reports = append(out.Reports, response.ReportResponse{
			ID:            r.ID.String(),
			UserID:        r.UserID.String(),
			ReportDate:    utils.GetTime(r.ReportDate),
			CompletedWork: complWork,
			PlanTomorrow:  plans,
			HelpRequest:   helpResp,
			CreatedAt:     utils.GetTime(r.CreatedAt),
			UpdatedAt:     utils.GetTime(r.UpdatedAt),
			UserInfo:      userInfo,
			Problems:      problemsResp,
			Checked:       utils.GetInt8(r.Checked),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// GetReport godoc
// @Summary Получение отчета по ID
// @Description Получает данные отчета по его уникальному идентификатору
// @Tags Reports
// @Accept json
// @Produce json
// @Param id path string true "ID отчета"
// @Security BearerAuth
// @Success 200 {object} response.ReportFullResponse "Отчет успешно получен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор отчета"
// @Failure 404 {object} map[string]string "Отчет не найден"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении отчета"
// @Router /report/{id} [get]
func (rc *ReportController) GetReport(c echo.Context) error {
	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор отчета"})
	}

	r, err := rc.reportService.GetReport(reportID)
	if err != nil {
		if err.Error() == "report not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Отчет не найден"})
		}
		log.Printf("service error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении отчета"})
	}

	userInfo := response.UserShort{
		ID:        r.UserID.String(),
		FirstName: r.User.FirstName,
		LastName:  r.User.LastName,
	}

	var complWork []response.CompletedWorkWithTask
	for _, w := range r.CompletedWork {
		task := response.TaskForReport{}
		if w.Task != nil {
			task.ID = w.Task.ID.String()
			task.Name = utils.GetString(w.Task.Name)
			task.Description = utils.GetString(w.Task.Description)

			if w.Task.Status != nil && w.Task.Status.Board != nil {
				task.Board = response.InTaskBoard{
					BoardId:   w.Task.Status.Board.ID.String(),
					BoardName: utils.GetString(w.Task.Status.Board.Name),
				}

				if w.Task.Status.Board.Project != nil {
					task.Project = response.InTaskProject{
						ProjectId:   w.Task.Status.Board.Project.ID.String(),
						ProjectName: utils.GetString(w.Task.Status.Board.Project.Name),
					}
				}
			}
		}
		complWork = append(complWork, response.CompletedWorkWithTask{
			ID:          w.ID.String(),
			Description: utils.GetString(w.Description),
			Task:        task,
		})
	}

	var plans []response.TomorrowPlansWithTask
	for _, p := range r.TomorrowPlans {
		task := response.TaskForReport{}
		if p.Task != nil {
			task.ID = p.Task.ID.String()
			task.Name = utils.GetString(p.Task.Name)
			task.Description = utils.GetString(p.Task.Description)

			if p.Task.Status != nil && p.Task.Status.Board != nil {
				task.Board = response.InTaskBoard{
					BoardId:   p.Task.Status.Board.ID.String(),
					BoardName: utils.GetString(p.Task.Status.Board.Name),
				}

				if p.Task.Status.Board.Project != nil {
					task.Project = response.InTaskProject{
						ProjectId:   p.Task.Status.Board.Project.ID.String(),
						ProjectName: utils.GetString(p.Task.Status.Board.Project.Name),
					}
				}
			}
		}
		plans = append(plans, response.TomorrowPlansWithTask{
			ID:          p.ID.String(),
			Description: utils.GetString(p.Description),
			Task:        task,
		})
	}

	var problemsResp []response.ProblemResponse
	for _, rp := range r.ReportProblems {
		if rp.Problem != nil {
			problemsResp = append(problemsResp, response.ProblemResponse{
				ID:          rp.Problem.ID.String(),
				Name:        utils.GetString(rp.Problem.Name),
				Description: rp.Problem.Description,
				CreatorId:   utils.GetUUIDString(rp.Problem.CreatorID),
				CreatedAt:   utils.GetTime(rp.Problem.CreatedAt),
				UpdatedAt:   utils.GetTime(rp.Problem.UpdatedAt),
			})
		}
	}

	helpResp := []response.HelpRequestItem{}
	for _, hr := range r.HelpRequests {
		helpResp = append(helpResp, response.HelpRequestItem{
			ID:          hr.ID.String(),
			HelperID:    utils.GetUUIDString(hr.HelperID),
			Description: utils.GetString(hr.Description),
			Status:      utils.GetString(hr.Status),
		})
	}

	return c.JSON(http.StatusOK, response.ReportFullResponse{
		ID:            r.ID.String(),
		UserID:        r.UserID.String(),
		ReportDate:    utils.GetTime(r.ReportDate),
		CompletedWork: complWork,
		PlanTomorrow:  plans,
		HelpRequest:   helpResp,
		CreatedAt:     utils.GetTime(r.CreatedAt),
		UpdatedAt:     utils.GetTime(r.UpdatedAt),
		UserInfo:      userInfo,
		Problems:      problemsResp,
		Checked:       utils.GetInt8(r.Checked),
	})
}

// GetReportsByTaskId godoc
// @Summary Получение отчетов по ID задачи
// @Description Получает список отчетов, связанных с указанной задачей
// @Tags Reports
// @Accept json
// @Produce json
// @Param id path string true "ID задачи"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ReportListByTaskId "Список отчетов успешно получен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор задачи"
// @Failure 404 {object} map[string]string "Пользователь, запрос на помощь, выполненная работа или план на завтра не найдены"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении отчетов"
// @Router /report/task/{id} [get]
func (rc *ReportController) GetReportsByTaskId(c echo.Context) error {
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный ID задачи"})
	}

	reports, err := rc.reportService.GetReportsByTaskId(taskID)
	if err != nil {
		log.Printf("service error (get reports by task): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении работ по задаче"})
	}

	out := response.ReportListByTaskId{TaskID: taskID.String()}
	for _, r := range reports {
		userInfo := response.UserShort{
			ID:        r.UserID.String(),
			FirstName: r.User.FirstName,
			LastName:  r.User.LastName,
			AvatarURL: rc.freshAvatarURL(r.User.AvatarURL),
		}
		var complWork []response.CompletedWork
		for _, w := range r.CompletedWork {
			complWork = append(complWork, response.CompletedWork{
				ID:          w.ID.String(),
				Description: utils.GetString(w.Description),
				TaskId:      utils.GetUUIDString(w.TaskID),
			})
		}
		var plans []response.TomorrowPlans
		for _, p := range r.TomorrowPlans {
			plans = append(plans, response.TomorrowPlans{
				ID:          p.ID.String(),
				Description: utils.GetString(p.Description),
				TaskId:      utils.GetUUIDString(p.TaskID),
			})
		}
		helpResp := []response.HelpRequestItem{}
		for _, hr := range r.HelpRequests {
			helpResp = append(helpResp, response.HelpRequestItem{
				ID:          hr.ID.String(),
				HelperID:    utils.GetUUIDString(hr.HelperID),
				Description: utils.GetString(hr.Description),
				Status:      utils.GetString(hr.Status),
			})
		}
		var problemsResp []response.ProblemResponse
		for _, rp := range r.ReportProblems {
			if rp.Problem != nil {
				problemsResp = append(problemsResp, response.ProblemResponse{
					ID:          rp.Problem.ID.String(),
					Name:        utils.GetString(rp.Problem.Name),
					Description: rp.Problem.Description,
					CreatorId:   utils.GetUUIDString(rp.Problem.CreatorID),
					CreatedAt:   utils.GetTime(rp.Problem.CreatedAt),
					UpdatedAt:   utils.GetTime(rp.Problem.UpdatedAt),
				})
			}
		}
		out.Reports = append(out.Reports, response.ReportResponse{
			ID:            r.ID.String(),
			UserID:        r.UserID.String(),
			ReportDate:    utils.GetTime(r.ReportDate),
			CompletedWork: complWork,
			PlanTomorrow:  plans,
			HelpRequest:   helpResp,
			CreatedAt:     utils.GetTime(r.CreatedAt),
			UpdatedAt:     utils.GetTime(r.UpdatedAt),
			UserInfo:      userInfo,
			Problems:      problemsResp,
			Checked:       utils.GetInt8(r.Checked),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// GetReportsByProjectId godoc
// @Summary Получение отчетов по ID проекта
// @Description Получает список отчетов, связанных с задачами указанного проекта
// @Tags Reports
// @Accept json
// @Produce json
// @Param id path string true "ID проекта"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ReportListByProjectId "Список отчетов успешно получен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор проекта"
// @Failure 404 {object} map[string]string "Пользователь, запрос на помощь, выполненная работа или план на завтра не найдены"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении отчетов"
// @Router /report/project/{id} [get]
func (rc *ReportController) GetReportsByProjectId(c echo.Context) error {
	projectUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проекта"})
	}

	reports, err := rc.reportService.GetReportsByProjectId(projectUUID)
	if err != nil {
		log.Printf("service error (get reports by project id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении задач"})
	}

	out := response.ReportListByProjectId{ProjectID: projectUUID.String()}
	for _, r := range reports {
		userInfo := response.UserShort{
			ID:        r.UserID.String(),
			FirstName: r.User.FirstName,
			LastName:  r.User.LastName,
			AvatarURL: rc.freshAvatarURL(r.User.AvatarURL),
		}
		var complWork []response.CompletedWork
		for _, w := range r.CompletedWork {
			complWork = append(complWork, response.CompletedWork{
				ID:          w.ID.String(),
				Description: utils.GetString(w.Description),
				TaskId:      utils.GetUUIDString(w.TaskID),
			})
		}
		var plans []response.TomorrowPlans
		for _, p := range r.TomorrowPlans {
			plans = append(plans, response.TomorrowPlans{
				ID:          p.ID.String(),
				Description: utils.GetString(p.Description),
				TaskId:      utils.GetUUIDString(p.TaskID),
			})
		}
		helpResp := []response.HelpRequestItem{}
		for _, hr := range r.HelpRequests {
			helpResp = append(helpResp, response.HelpRequestItem{
				ID:          hr.ID.String(),
				HelperID:    utils.GetUUIDString(hr.HelperID),
				Description: utils.GetString(hr.Description),
				Status:      utils.GetString(hr.Status),
			})
		}
		var problemsResp []response.ProblemResponse
		for _, rp := range r.ReportProblems {
			if rp.Problem != nil {
				problemsResp = append(problemsResp, response.ProblemResponse{
					ID:          rp.Problem.ID.String(),
					Name:        utils.GetString(rp.Problem.Name),
					Description: rp.Problem.Description,
					CreatorId:   utils.GetUUIDString(rp.Problem.CreatorID),
					CreatedAt:   utils.GetTime(rp.Problem.CreatedAt),
					UpdatedAt:   utils.GetTime(rp.Problem.UpdatedAt),
				})
			}
		}
		out.Reports = append(out.Reports, response.ReportResponse{
			ID:            r.ID.String(),
			UserID:        r.UserID.String(),
			ReportDate:    utils.GetTime(r.ReportDate),
			CompletedWork: complWork,
			PlanTomorrow:  plans,
			HelpRequest:   helpResp,
			CreatedAt:     utils.GetTime(r.CreatedAt),
			UpdatedAt:     utils.GetTime(r.UpdatedAt),
			UserInfo:      userInfo,
			Problems:      problemsResp,
			Checked:       utils.GetInt8(r.Checked),
		})
	}

	return c.JSON(http.StatusOK, out)
}

// CreateReport godoc
// @Summary Создание нового отчета
// @Description Создает новый отчет с указанными параметрами, включая выполненную работу, планы на завтра, проблемы и запрос на помощь
// @Tags Reports
// @Accept json
// @Produce json
// @Param report body request.ReportCreateRequest true "Данные для создания отчета"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 201 {object} response.ReportUniversalResponse "Отчет успешно создан"
// @Failure 400 {object} map[string]string "Ошибка в запросе или некорректные идентификаторы"
// @Failure 500 {object} map[string]string "Ошибка сервера при создании отчета"
// @Router /report [post]
func (rc *ReportController) CreateReport(c echo.Context) error {
	var req request.ReportCreateRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	reportID, err := rc.reportService.CreateReport(req)
	if err != nil {
		if err.Error() == "invalid user id" {
			log.Printf("ParseUserId error: %v", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось распарсить userId"})
		}
		log.Printf("service error (create report): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при создании отчета"})
	}

	return c.JSON(http.StatusCreated, response.ReportUniversalResponse{
		ID:      reportID.String(),
		Message: "Успешно создано",
	})
}

// UpdateReport godoc
// @Summary Обновление отчета и связанных данных, ЕСЛИ НЕ УКАЗЫВАТЬ ID ВО ВСПОМОГАТЕЛЬНЫХ СУЩНОСТЯХ, СОЗДАЕТ НОВЫЕ
// @Tags Reports
// @Description Обновляет поля отчета, а также связанные CompletedWork, TomorrowPlans, HelpRequests и ReportProblems, ЕСЛИ НЕ УКАЗЫВАТЬ ID ВО ВСПОМОГАТЕЛЬНЫХ СУЩНОСТЯХ, СОЗДАЕТ НОВЫЕ
// @Accept json
// @Produce json
// @Param id path string true "ID отчета"
// @Param report body request.ReportReplaceRequest true "Данные для обновления отчета и связанных сущностей"
// @Security BearerAuth
// @Success 200 {object} response.ReportUniversalResponse "Отчет успешно обновлен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор отчета или ошибка в запросе"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 404 {object} map[string]string "Отчет не найден"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении отчета"
// @Router /report/{id} [patch]
func (rc *ReportController) UpdateReport(c echo.Context) error {
	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор отчета"})
	}
	var req request.ReportReplaceRequest
	if err = c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	err = rc.reportService.UpdateReport(reportID, req)
	if err != nil {
		if err.Error() == "report not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Отчет не найден"})
		}
		log.Printf("service error (update report): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении отчета"})
	}

	return c.JSON(http.StatusOK, response.ReportUniversalResponse{
		ID:      reportID.String(),
		Message: "Отчет успешно изменен",
	})
}

// DeleteReport godoc
// @Summary Удаление отчета
// @Description Логическое удаление отчета по ID, включая связанные данные (поле deleted = true)
// @Tags Reports
// @Accept json
// @Produce json
// @Param id path string true "ID отчета"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ReportUniversalResponse "Отчет успешно удален"
// @Failure 404 {object} map[string]string "Отчет не найден"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении отчета"
// @Router /report/{id} [delete]
func (rc *ReportController) DeleteReport(c echo.Context) error {
	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор отчета"})
	}

	err = rc.reportService.DeleteReport(reportID)
	if err != nil {
		if err.Error() == "report not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (delete report): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении отчета"})
	}

	return c.JSON(http.StatusOK, response.ReportUniversalResponse{
		ID:      reportID.String(),
		Message: "Отчет успешно удален",
	})
}

// UpdateHelpRequest godoc
// @Summary Обновление запроса на помощь
// @Description Обновляет данные запроса на помощь по его ID
// @Tags Reports
// @Accept json
// @Produce json
// @Param id path string true "ID запроса на помощь"
// @Param helpRequest body request.HelpRequestUpdateRequest true "Данные для обновления запроса на помощь"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ReportUniversalResponse "Запрос на помощь успешно обновлен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор или ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении запроса на помощь"
// @Router /report/help-request/{id} [patch]
func (rc *ReportController) UpdateHelpRequest(c echo.Context) error {
	helpID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор запроса помощи"})
	}
	var req request.HelpRequestUpdateRequest
	if err = c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	err = rc.reportService.UpdateHelpRequest(helpID, req)
	if err != nil {
		if err.Error() == "no fields to update" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не указаны поля для обновления"})
		}
		if err.Error() == "help request not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не обновлено"})
		}
		log.Printf("service error (update help request): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении запроса на помощь"})
	}

	return c.JSON(http.StatusOK, response.ReportUniversalResponse{
		ID:      helpID.String(),
		Message: "Запрос на помощь обновлен",
	})
}

// UpdateCompletedWork godoc
// @Summary Обновление выполненной работы
// @Description Обновляет данные выполненной работы по ее ID
// @Tags Reports
// @Accept json
// @Produce json
// @Param id path string true "ID выполненной работы"
// @Param completedWork body request.CompletedWorkUpdateRequest true "Данные для обновления выполненной работы"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ReportUniversalResponse "Выполненная работа успешно обновлена"
// @Failure 400 {object} map[string]string "Некорректный идентификатор или ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении выполненной работы"
// @Router /report/completed-work/{id} [patch]
func (rc *ReportController) UpdateCompletedWork(c echo.Context) error {
	cwID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор выполненной работы"})
	}
	var req request.CompletedWorkUpdateRequest
	if err = c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	err = rc.reportService.UpdateCompletedWork(cwID, req)
	if err != nil {
		if err.Error() == "no fields to update" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не указаны поля для обновления"})
		}
		if err.Error() == "completed work not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не обновлено"})
		}
		log.Printf("service error (update completed work): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении проделанной работы"})
	}

	return c.JSON(http.StatusOK, response.ReportUniversalResponse{
		ID:      cwID.String(),
		Message: "Выполненная работа обновлена",
	})
}

// UpdateTomorrowPlans godoc
// @Summary Обновление планов на завтра
// @Description Обновляет данные планов на завтра по их ID
// @Tags Reports
// @Accept json
// @Produce json
// @Param id path string true "ID планов на завтра"
// @Param tomorrowPlans body request.TomorrowPlansUpdateRequest true "Данные для обновления планов на завтра"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ReportUniversalResponse "Планы на завтра успешно обновлены"
// @Failure 400 {object} map[string]string "Некорректный идентификатор или ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении планов на завтра"
// @Router /report/tomorrow-plans/{id} [patch]
func (rc *ReportController) UpdateTomorrowPlans(c echo.Context) error {
	tpID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор планов на завтра"})
	}
	var req request.TomorrowPlansUpdateRequest
	if err = c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	err = rc.reportService.UpdateTomorrowPlans(tpID, req)
	if err != nil {
		if err.Error() == "no fields to update" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не указаны поля для обновления"})
		}
		if err.Error() == "tomorrow plans not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не обновлено"})
		}
		log.Printf("service error (update tomorrow plans): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении планов на завтра"})
	}

	return c.JSON(http.StatusOK, response.ReportUniversalResponse{
		ID:      tpID.String(),
		Message: "Планы на завтра обновлены",
	})
}

// GetHelpRequestsForUser godoc
// @Summary Получение запросов на помощь по ID пользователя-помощника
// @Description Возвращает список запросов на помощь, где указанный пользователь назначен в качестве помощника. В ответе также возвращаются имя и фамилия пользователя, создавшего запрос (автора ежедневного отчёта).
// @Tags Reports
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя-помощника (в формате UUID)"
// @Security BearerAuth
// @Success 200 {object} response.HelpRequestsForUser "Список запросов на помощь успешно получен"
// @Failure 400 {object} map[string]string "Некорректный ID пользователя"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении запросов на помощь"
// @Router /report/help-requests-by-user-id/{id} [get]
func (rc *ReportController) GetHelpRequestsForUser(c echo.Context) error {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный ID пользователя"})
	}

	helpRequests, err := rc.reportService.GetHelpRequestsForUser(userID)
	if err != nil {
		log.Printf("service error (find help_requests): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении запросов на помощь"})
	}

	helpRequestResponse := response.HelpRequestsForUser{}
	for _, helpRequest := range helpRequests {
		var firstName, lastName string
		if helpRequest.Report != nil && helpRequest.Report.User != nil {
			firstName = helpRequest.Report.User.FirstName
			lastName = helpRequest.Report.User.LastName
		}
		helpRequestResp := response.HelpRequestWithAssignerID{
			HelpRequest: response.HelpRequestItem{
				ID:          helpRequest.ID.String(),
				HelperID:    utils.GetUUIDString(helpRequest.HelperID),
				Description: utils.GetString(helpRequest.Description),
				Status:      utils.GetString(helpRequest.Status),
			},
			UserFirstName: firstName,
			UserLastName:  lastName,
		}
		helpRequestResponse.HelpRequests = append(helpRequestResponse.HelpRequests, helpRequestResp)
	}
	return c.JSON(http.StatusOK, helpRequestResponse)
}

// DeleteHelpRequest godoc
// @Summary Удаление запроса на помощь
// @Description Логическое удаление запроса на помощь по ID (установка поля deleted = true)
// @Tags Reports
// @Accept json
// @Produce json
// @Param id path string true "ID запроса на помощь"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ReportUniversalResponse "Запрос на помощь успешно удален"
// @Failure 400 {object} map[string]string "Некорректный идентификатор запроса на помощь"
// @Failure 404 {object} map[string]string "Запрос на помощь не найден"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении запроса на помощь"
// @Router /report/help-request/{id} [delete]
func (rc *ReportController) DeleteHelpRequest(c echo.Context) error {
	requestID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор запроса на помощь"})
	}

	err = rc.reportService.DeleteHelpRequest(requestID)
	if err != nil {
		if err.Error() == "help request not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (delete help request): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении запроса на помощь"})
	}

	return c.JSON(http.StatusOK, response.ReportUniversalResponse{
		ID:      requestID.String(),
		Message: "Запрос на помощь успешно удален",
	})
}

// GetReportByDateInXLSX godoc
// @Summary Экспорт отчётов по выполненным работам в XLSX
// @Description Генерирует XLSX-файл с отчётами по выполненным работам за указанный период.
// @Description Строки — сотрудники, столбцы — даты, ячейки — список выполненных задач за день.
// @Tags Reports
// @Accept json
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param request body request.ReportsByDateInXLSX true "Диапазон дат для экспорта"
// @Security BearerAuth
// @Success 200 {string} file "XLSX-файл с отчётами"
// @Failure 400 {object} map[string]string "Некорректные входные данные (например, даты)"
// @Failure 401 {object} map[string]string "Нет или неверный токен авторизации"
// @Failure 500 {object} map[string]string "Ошибка при генерации отчёта"
// @Router /report/export/xlsx [post]
func (rc *ReportController) GetReportByDateInXLSX(c echo.Context) error {
	var req request.ReportsByDateInXLSX
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	data, err := rc.reportService.GetReportByDateInXLSX(req)
	if err != nil {
		log.Printf("service error (get report data for XLSX): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подготовке данных"})
	}

	f := excelize.NewFile()
	sheet := "Отчёты"
	f.SetSheetName(f.GetSheetName(0), sheet)

	headers := []interface{}{"Сотрудник"}
	for _, d := range data.Dates {
		headers = append(headers, d.Format("02.01.2006"))
	}
	if err := f.SetSheetRow(sheet, "A1", &headers); err != nil {
		log.Printf("XLSX header error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при создании XLSX"})
	}

	for rowIdx, userName := range data.Users {
		row := make([]interface{}, len(data.Dates)+1)
		row[0] = userName

		for colIdx, date := range data.Dates {
			works := data.Grid[userName][date]
			if len(works) == 0 {
				row[colIdx+1] = ""
			} else {
				row[colIdx+1] = strings.Join(works, "\n")
			}
		}

		axis := fmt.Sprintf("A%d", rowIdx+2)
		if err := f.SetSheetRow(sheet, axis, &row); err != nil {
			log.Printf("XLSX row error: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при заполнении XLSX"})
		}
	}

	if len(data.Users) > 0 {
		for i := 1; i <= len(data.Dates)+1; i++ {
			colName, _ := excelize.ColumnNumberToName(i)
			f.SetColWidth(sheet, colName, colName, 30)
		}

		styleID, _ := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
		})
		lastRow := len(data.Users) + 1
		lastCol, _ := excelize.ColumnNumberToName(len(data.Dates) + 1)
		f.SetCellStyle(sheet, "B2", lastCol+strconv.Itoa(lastRow), styleID)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		log.Printf("XLSX buffer error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при генерации файла"})
	}

	filename := fmt.Sprintf("reports_%s_%s.xlsx",
		req.StartDate.Format("2006-01-02"),
		req.EndDate.Format("2006-01-02"))

	c.Response().Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Response().Header().Set("Content-Disposition", "attachment; filename="+filename)
	return c.Blob(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}
