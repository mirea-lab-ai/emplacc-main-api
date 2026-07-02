package httpapi

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/service"
	"emplacc-api/internal/utils"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// forumMentionRe — mention-маркап @[Имя](type:id) для очистки превью цитаты.
var forumMentionRe = regexp.MustCompile(`[@#]\[([^\]]+)\]\((?:user|task|project|team):[^)]+\)`)

// presignedURLRe — presigned S3-ссылка (с X-Amz-Signature) внутри текста сообщения.
var presignedURLRe = regexp.MustCompile(`https?://[^\s)"'<>]*X-Amz-Signature=[^\s)"'<>]+`)

// freshenContentURLs переподписывает presigned-ссылки на вложения (картинки/файлы) прямо в
// тексте сообщения — клиенту приходят уже свежие ссылки (как freshAvatarURL для аватаров),
// поэтому истёкшие вложения не «залипают» битыми и клиенту ничего рефрешить не нужно.
func freshenContentURLs(lines []string, fresh func(string) string) []string {
	if fresh == nil {
		return lines
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = presignedURLRe.ReplaceAllStringFunc(line, func(u string) string {
			if nu := fresh(u); nu != "" {
				return nu
			}
			return u
		})
	}
	return out
}

// forumReplyPreview строит превью цитируемого сообщения: убирает mention-маркап и
// обрезает по РУНАМ (не байтам), иначе кириллица рвётся на � при срезе.
func forumReplyPreview(desc []string) string {
	text := forumMentionRe.ReplaceAllString(strings.Join(desc, " "), "@$1")
	r := []rune(strings.TrimSpace(text))
	if len(r) > 100 {
		return string(r[:100]) + "…"
	}
	return string(r)
}

type ForumMessageController struct {
	forumMessageService service.ForumMessageService
	freshAvatarURL      func(string) string
}

func NewForumMessageController(forumMessageService service.ForumMessageService, freshAvatarURL func(string) string) *ForumMessageController {
	return &ForumMessageController{
		forumMessageService: forumMessageService,
		freshAvatarURL:      freshAvatarURL,
	}
}

func RegisterForumMessagesRoutes(e Router, forumMessageService service.ForumMessageService, freshAvatarURL func(string) string, employeeMw echo.MiddlewareFunc, managerMw echo.MiddlewareFunc) {
	controller := NewForumMessageController(forumMessageService, freshAvatarURL)
	g := e.Group("/forum-messages")
	g.GET("/all/:page/:pagesize", controller.GetAllForumMessages)
	g.GET("/problem/:id/:page/:pagesize", controller.GetForumMessagesByProblemId)
	g.GET("/:id", controller.GetForumMessageById)
	g.POST("", controller.CreateForumMessage, employeeMw)
	g.PATCH("/:id", controller.UpdateForumMessage, employeeMw)
	g.DELETE("/:id", controller.DeleteForumMessage, employeeMw) // ownership check inside
}

func buildForumMessageResponse(m models.ForumMessage, freshAvatarURL func(string) string) response.ForumMessageResponse {
	r := response.ForumMessageResponse{
		ID:          m.ID.String(),
		ProblemID:   m.ProblemID.String(),
		Description: freshenContentURLs([]string(m.Description), freshAvatarURL),
		CreatorID:   utils.GetUUIDString(m.CreatorID),
		CreatedAt:   utils.GetTime(m.CreatedAt),
		UpdatedAt:   utils.GetTime(m.UpdatedAt),
	}

	if m.ReplyToID != nil {
		s := m.ReplyToID.String()
		r.ReplyToID = &s
	}

	if m.User != nil {
		r.Author = &response.ForumMessageAuthor{
			ID:        m.User.ID.String(),
			FirstName: m.User.FirstName,
			LastName:  m.User.LastName,
			AvatarURL: freshAvatarURL(m.User.AvatarURL),
			Email:     m.User.Email,
		}
	}

	if m.ReplyTo != nil && (m.ReplyTo.Deleted == nil || !*m.ReplyTo.Deleted) {
		text := forumReplyPreview([]string(m.ReplyTo.Description))
		authorName := ""
		if m.ReplyTo.User != nil {
			authorName = strings.TrimSpace(m.ReplyTo.User.FirstName + " " + m.ReplyTo.User.LastName)
		}
		r.ReplyTo = &response.ForumMessageReplyPreview{
			ID:         m.ReplyTo.ID.String(),
			Text:       text,
			AuthorName: authorName,
		}
	}

	return r
}

func (fmc *ForumMessageController) GetAllForumMessages(c echo.Context) error {
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге страницы"})
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге размера страницы"})
	}

	forumMessages, totalCount, err := fmc.forumMessageService.GetAllForumMessages(page, pageSize)
	if err != nil {
		log.Printf("service error (get all forum messages): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подсчете сообщений форума"})
	}

	out := response.ForumMessageListResponse{Page: page, PageSize: pageSize, TotalCount: totalCount}
	for _, m := range forumMessages {
		out.Messages = append(out.Messages, buildForumMessageResponse(m, fmc.freshAvatarURL))
	}
	return c.JSON(http.StatusOK, out)
}

func (fmc *ForumMessageController) GetForumMessagesByProblemId(c echo.Context) error {
	problemID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Неверный формат идентификатора проблемы"})
	}
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге страницы"})
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге размера страницы"})
	}

	forumMessages, totalCount, err := fmc.forumMessageService.GetForumMessagesByProblemId(problemID, page, pageSize)
	if err != nil {
		log.Printf("service error (get forum messages by problem id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении сообщений форума"})
	}

	out := response.ForumMessageListByProblemIdResponse{
		ProblemId: problemID.String(), Page: page, PageSize: pageSize, TotalCount: totalCount,
	}
	for _, m := range forumMessages {
		out.Messages = append(out.Messages, buildForumMessageResponse(m, fmc.freshAvatarURL))
	}
	return c.JSON(http.StatusOK, out)
}

func (fmc *ForumMessageController) GetForumMessageById(c echo.Context) error {
	messageID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Неверный формат идентификатора сообщения"})
	}
	m, err := fmc.forumMessageService.GetForumMessageById(messageID)
	if err != nil {
		if err.Error() == "forum message not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Сообщение не найдено"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении сообщения"})
	}
	return c.JSON(http.StatusOK, buildForumMessageResponse(*m, fmc.freshAvatarURL))
}

func (fmc *ForumMessageController) CreateForumMessage(c echo.Context) error {
	var req request.CreateForumMessageRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}
	messageID, err := fmc.forumMessageService.CreateForumMessage(req)
	if err != nil {
		switch err.Error() {
		case "invalid problem id":
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проблемы"})
		case "invalid creator id":
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор создателя"})
		default:
			log.Printf("service error (create forum message): %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при создании сообщения"})
		}
	}
	return c.JSON(http.StatusCreated, response.ForumMessageUniversalResponse{ID: messageID.String(), Message: "Сообщение создано"})
}

func (fmc *ForumMessageController) UpdateForumMessage(c echo.Context) error {
	messageID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Неверный формат идентификатора сообщения"})
	}

	// Только владелец может редактировать
	userIDStr, _ := c.Get("user_id").(string)
	m, err := fmc.forumMessageService.GetForumMessageById(messageID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Сообщение не найдено"})
	}
	if m.CreatorID == nil || m.CreatorID.String() != userIDStr {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Можно редактировать только свои сообщения"})
	}

	var req request.UpdateForumMessageRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}
	err = fmc.forumMessageService.UpdateForumMessage(messageID, req)
	if err != nil {
		switch err.Error() {
		case "no fields to update":
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не указаны поля для обновления"})
		case "forum message not found":
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не обновлено"})
		default:
			log.Printf("service error (update forum message): %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении сообщения"})
		}
	}
	return c.JSON(http.StatusOK, response.ForumMessageUniversalResponse{ID: messageID.String(), Message: "Сообщение обновлено"})
}

func (fmc *ForumMessageController) DeleteForumMessage(c echo.Context) error {
	messageID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Неверный формат идентификатора сообщения"})
	}

	userIDStr, _ := c.Get("user_id").(string)
	userRole, _ := c.Get("user_role").(string)

	// Получаем сообщение для проверки владельца
	m, err := fmc.forumMessageService.GetForumMessageById(messageID)
	if err != nil {
		if err.Error() == "forum message not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении сообщения"})
	}

	// Менеджер/администратор может удалять любые сообщения; остальные — только свои
	isPrivileged := userRole == "manager" || userRole == "admin"
	isOwner := m.CreatorID != nil && m.CreatorID.String() == userIDStr
	if !isPrivileged && !isOwner {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Можно удалять только свои сообщения"})
	}

	err = fmc.forumMessageService.DeleteForumMessage(messageID)
	if err != nil {
		if err.Error() == "forum message not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (delete forum message): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении сообщения"})
	}
	return c.JSON(http.StatusOK, response.ForumMessageUniversalResponse{ID: messageID.String(), Message: "Сообщение удалено"})
}
