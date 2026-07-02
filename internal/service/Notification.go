package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"errors"
	"fmt"
	"html"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

type NotificationService interface {
	ports.Notifier
	List(userID uuid.UUID, page, pageSize int, onlyUnread bool) ([]models.Notification, int64, error)
	UnreadCount(userID uuid.UUID) (int64, error)
	MarkRead(id, userID uuid.UUID) error
	MarkAllRead(userID uuid.UUID) error
	GetEmailPref(userID uuid.UUID) (bool, error)
	SetEmailPref(userID uuid.UUID, enabled bool) error
}

type notificationService struct {
	repo     ports.NotificationRepository
	userRepo ports.UserRepository
	mailer   ports.Mailer  // optional (nil → email disabled)
	emailSem chan struct{} // ограничивает число одновременных email-горутин
	webBase  string        // публичный URL фронта для ссылок в письмах
}

func NewNotificationService(repo ports.NotificationRepository, userRepo ports.UserRepository, mailer ports.Mailer) NotificationService {
	base := strings.TrimRight(os.Getenv("WEB_BASE_URL"), "/")
	if base == "" {
		base = "https://emplacc.g-309.ru"
	}
	return &notificationService{
		repo:     repo,
		userRepo: userRepo,
		mailer:   mailer,
		emailSem: make(chan struct{}, 8),
		webBase:  base,
	}
}

// Notify создаёт уведомление и шлёт realtime-сигнал. Best-effort: ошибка БД не
// должна валить вызывающую бизнес-операцию — логируем и продолжаем.
func (s *notificationService) Notify(userID uuid.UUID, typ, title, body, entityType string, entityID *uuid.UUID) {
	if userID == uuid.Nil {
		return
	}
	n := &models.Notification{
		ID:         uuid.New(),
		UserID:     userID,
		Type:       typ,
		Title:      title,
		Body:       body,
		EntityType: entityType,
		EntityID:   entityID,
		Read:       false,
		CreatedAt:  time.Now(),
	}
	if err := s.repo.Create(n); err != nil {
		log.Printf("notify: create failed (non-fatal): %v", err)
		return
	}
	// Сигнал клиенту: «у тебя что-то изменилось — дозапроси свой счётчик/список».
	publishGlobal(StreamEvent{Type: "notification", WorkItemID: userID.String()})

	// Email — best-effort, в фоне, чтобы не блокировать бизнес-операцию на SMTP.
	// Семафор ограничивает число одновременных отправок; при перегрузе письмо
	// тихо пропускаем (in-app/SSE уже доставлены).
	if s.mailer != nil && s.userRepo != nil {
		select {
		case s.emailSem <- struct{}{}:
			go func() {
				defer func() { <-s.emailSem }()
				s.sendEmail(userID, title, body, entityType, entityID)
			}()
		default:
			log.Printf("notify email: queue full, skipping email for %s", userID)
		}
	}
}

func (s *notificationService) sendEmail(userID uuid.UUID, title, body, entityType string, entityID *uuid.UUID) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("notify email: panic recovered: %v", r)
		}
	}()
	user, err := s.userRepo.GetUserById(userID)
	if err != nil || user == nil || user.Email == "" {
		return
	}
	// Преференция пользователя: nil = включено (старые записи).
	if user.EmailNotifications != nil && !*user.EmailNotifications {
		return
	}
	link, cta := s.notificationLink(entityType, entityID)
	htmlBody := notificationEmailHTML(s.webBase, title, body, link, cta)
	if err := s.mailer.Send(user.Email, title, htmlBody); err != nil {
		log.Printf("notify email: send to %s failed (non-fatal): %v", user.Email, err)
	}
}

// notificationLink строит deep-link на источник уведомления и подпись кнопки.
func (s *notificationService) notificationLink(entityType string, entityID *uuid.UUID) (link, cta string) {
	id := ""
	if entityID != nil {
		id = entityID.String()
	}
	switch {
	case entityType == "task" && id != "":
		return s.webBase + "/tasks/" + id, "Открыть задачу"
	case entityType == "problem" && id != "":
		return s.webBase + "/forum?problem=" + id, "Открыть обсуждение"
	case entityType == "project" && id != "":
		return s.webBase + "/projects/" + id, "Открыть проект"
	default:
		return s.webBase, "Открыть Emplacc"
	}
}

// notificationEmailHTML — адаптивное HTML-письмо (таблицы + инлайн-стили для
// совместимости с почтовыми клиентами): шапка с логотипом, заголовок, текст,
// кнопка на источник, футер.
func notificationEmailHTML(base, title, body, link, cta string) string {
	safeTitle := html.EscapeString(title)
	safeBody := html.EscapeString(body)
	if strings.TrimSpace(safeBody) == "" {
		safeBody = safeTitle
	}
	safeBody = strings.ReplaceAll(safeBody, "\n", "<br>")
	logo := base + "/favicon.png"
	return fmt.Sprintf(`<!doctype html><html><body style="margin:0;padding:0;background:#eef2f0;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#eef2f0;padding:24px 12px;font-family:Arial,Helvetica,sans-serif;">
<tr><td align="center">
<table role="presentation" width="480" cellpadding="0" cellspacing="0" style="max-width:480px;width:100%%;background:#ffffff;border:1px solid #dfe5e2;border-radius:12px;overflow:hidden;">
<tr><td style="background:#0f1d16;padding:16px 24px;">
<img src="%s" width="26" height="26" alt="Emplacc" style="vertical-align:middle;border:0;border-radius:6px;">
<span style="color:#4ade80;font-size:18px;font-weight:bold;vertical-align:middle;margin-left:8px;">Emplacc</span>
</td></tr>
<tr><td style="padding:28px 24px 6px;"><h1 style="margin:0;font-size:20px;line-height:1.3;color:#10231a;">%s</h1></td></tr>
<tr><td style="padding:4px 24px 22px;color:#42514a;font-size:15px;line-height:1.6;">%s</td></tr>
<tr><td style="padding:0 24px 30px;">
<a href="%s" style="display:inline-block;background:#16a34a;color:#ffffff;text-decoration:none;font-size:15px;font-weight:bold;padding:12px 24px;border-radius:8px;">%s &rarr;</a>
</td></tr>
<tr><td style="padding:16px 24px;background:#f5f7f6;border-top:1px solid #dfe5e2;color:#8a958f;font-size:12px;line-height:1.5;">
Автоматическое уведомление Emplacc. Управлять — в <a href="%s/settings" style="color:#16a34a;text-decoration:none;">настройках</a>.
</td></tr>
</table>
</td></tr>
</table>
</body></html>`, logo, safeTitle, safeBody, link, html.EscapeString(cta), base)
}

// GetEmailPref — включены ли email-уведомления у пользователя (nil = да).
func (s *notificationService) GetEmailPref(userID uuid.UUID) (bool, error) {
	user, err := s.userRepo.GetUserById(userID)
	if err != nil || user == nil {
		return true, err
	}
	return user.EmailNotifications == nil || *user.EmailNotifications, nil
}

func (s *notificationService) SetEmailPref(userID uuid.UUID, enabled bool) error {
	_, err := s.userRepo.UpdateUser(userID, map[string]interface{}{"email_notifications": enabled})
	return err
}

func (s *notificationService) List(userID uuid.UUID, page, pageSize int, onlyUnread bool) ([]models.Notification, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 30
	}
	return s.repo.ListByUser(userID, pageSize, (page-1)*pageSize, onlyUnread)
}

func (s *notificationService) UnreadCount(userID uuid.UUID) (int64, error) {
	return s.repo.UnreadCount(userID)
}

func (s *notificationService) MarkRead(id, userID uuid.UUID) error {
	ok, err := s.repo.MarkRead(id, userID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("notification not found")
	}
	return nil
}

func (s *notificationService) MarkAllRead(userID uuid.UUID) error {
	return s.repo.MarkAllRead(userID)
}
