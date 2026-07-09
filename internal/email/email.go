package email

import (
	"errors"
	"fmt"
	"mime"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strings"
)

var ErrNotConfigured = errors.New("email sender is not configured")

type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

type Sender struct {
	cfg Config
}

func NewSender(cfg Config) *Sender {
	return &Sender{cfg: cfg}
}

func (sender *Sender) Configured() bool {
	return strings.TrimSpace(sender.cfg.Host) != "" && strings.TrimSpace(sender.cfg.From) != ""
}

func (sender *Sender) Send(to string, subject string, body string) error {
	if !sender.Configured() {
		return ErrNotConfigured
	}

	from, err := netmail.ParseAddress(sender.cfg.From)
	if err != nil {
		return fmt.Errorf("parse sender address: %w", err)
	}

	recipient, err := netmail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("parse recipient address: %w", err)
	}

	host := strings.TrimSpace(sender.cfg.Host)
	port := strings.TrimSpace(sender.cfg.Port)
	if port == "" {
		port = "587"
	}

	var auth smtp.Auth
	if sender.cfg.Username != "" || sender.cfg.Password != "" {
		auth = loginAuth{
			username: sender.cfg.Username,
			password: sender.cfg.Password,
		}
	}

	message := strings.Join([]string{
		"From: " + from.String(),
		"To: " + recipient.String(),
		"Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		body,
	}, "\r\n")

	if err := smtp.SendMail(net.JoinHostPort(host, port), auth, from.Address, []string{recipient.Address}, []byte(message)); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

type loginAuth struct {
	username string
	password string
}

func (auth loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (auth loginAuth) Next(challenge []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}

	prompt := strings.ToLower(string(challenge))
	switch {
	case strings.Contains(prompt, "username"):
		return []byte(auth.username), nil
	case strings.Contains(prompt, "password"):
		return []byte(auth.password), nil
	default:
		return []byte(auth.username), nil
	}
}
