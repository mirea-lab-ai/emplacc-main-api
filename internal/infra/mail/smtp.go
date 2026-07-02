// Package mail is the outbound SMTP adapter (implements ports.Mailer).
// net/smtp stays confined here; nothing above the adapter layer imports it.
package mail

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"

	"emplacc-api/internal/ports"
)

const (
	dialTimeout = 10 * time.Second
	sendTimeout = 20 * time.Second
)

type smtpMailer struct {
	addr     string // host:port
	host     string
	from     string
	username string
	password string
}

// New builds the SMTP mailer from SMTP_* env vars. Returns nil (no-op mailer)
// when SMTP_HOST is unset, so notifications degrade gracefully without email.
func New() ports.Mailer {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return nil
	}
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "587"
	}
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		from = os.Getenv("SMTP_USERNAME")
	}
	return &smtpMailer{
		addr:     host + ":" + port,
		host:     host,
		from:     from,
		username: os.Getenv("SMTP_USERNAME"),
		password: os.Getenv("SMTP_PASSWORD"),
	}
}

// Send отправляет письмо с явными таймаутами на dial и весь обмен, чтобы зависший
// SMTP не держал горутину бесконечно (вместо smtp.SendMail без дедлайнов).
func (m *smtpMailer) Send(to, subject, body string) error {
	if to == "" {
		return fmt.Errorf("empty recipient")
	}

	conn, err := net.DialTimeout("tcp", m.addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("dial %s: %w", m.addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(sendTimeout))

	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: m.host}); err != nil {
			return err
		}
	}
	if m.username != "" {
		if err := c.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
			return err
		}
	}
	if err := c.Mail(m.from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		// RFC 2047 encoded-word — иначе кириллическая тема бьётся в части клиентов.
		"Subject: =?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?=",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		body,
	}, "\r\n")
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
