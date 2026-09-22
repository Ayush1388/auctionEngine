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

	message := []byte(
		"From: " + s.from + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" +
			body,
	)

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
