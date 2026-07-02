package ports

// Mailer — выходной порт отправки писем. Реализация-адаптер в internal/infra/mail.
// Может отсутствовать (nil) — тогда NotificationService просто не шлёт email.
type Mailer interface {
	Send(to, subject, body string) error
}
