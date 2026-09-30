// Package email sends transactional email over SMTP.
//
// Nothing calls SendActivationEmail from a request handler. Emails are
// queued in the outbox inside the same database transaction as the change
// that caused them, and the outbox worker calls the EventHandler in
// handler.go. See docs/decisions/0001-transactional-outbox.md.
package email

import (
	"fmt"
	"net/smtp"
	"strconv"
)

type Service struct {
	host     string
	port     int
	username string
	password string
	from     string
	baseURL  string
}

func NewService(
	host string,
	port int,
	username string,
	password string,
	from string,
	baseURL string,
) *Service {
	return &Service{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		baseURL:  baseURL,
	}
}

func (s *Service) SendActivationEmail(
	to string,
	activationToken string,
) error {
	subject := "Activate your Auction Engine account"

	// The token is base64url, which is already safe inside a URL.
	activationURL := fmt.Sprintf(
		"%s/v1/users/activate?token=%s",
		s.baseURL,
		activationToken,
	)

	body := fmt.Sprintf(
		"Welcome!\n\n"+
			"Activate your Auction Engine account using this link:\n\n"+
			"%s\n\n"+
			"This link will expire in 24 hours.",
		activationURL,
	)

	// A raw RFC 5322 message: headers, a blank line, then the body.
	// "to" comes from user input, but it passed email validation at
	// registration, which rejects \r and \n, so it cannot inject extra
	// headers (a classic SMTP header-injection attack).
	message := []byte(
		"From: " + s.from + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" +
			body,
	)

	// Local dev servers (MailHog, Mailtrap) may not need credentials.
	var auth smtp.Auth

	if s.username != "" && s.password != "" {
		auth = smtp.PlainAuth(
			"",
			s.username,
			s.password,
			s.host,
		)
	}

	address := s.host + ":" + strconv.Itoa(s.port)

	if err := smtp.SendMail(
		address,
		auth,
		s.from,
		[]string{to},
		message,
	); err != nil {
		return fmt.Errorf("failed to send activation email: %w", err)
	}

	return nil
}
