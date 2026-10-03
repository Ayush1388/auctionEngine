// Package email sends transactional email over SMTP.
//
// Nothing calls SendActivationEmail from a request handler. Emails are
// queued in the outbox inside the same database transaction as the change
// that caused them, and the outbox worker calls the EventHandler in
// handler.go. See docs/decisions/0001-transactional-outbox.md.
package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/breaker"
)

// SendTimeout bounds one whole SMTP conversation (v1.0).
//
// smtp.SendMail, used before v1.0, has no timeout at all: it dials without
// one and never sets a deadline on the connection. An SMTP server that
// accepts the TCP connection and then hangs would block the outbox worker
// forever, and with it every other event behind the email (search
// indexing, cache invalidation, settlement). One stuck dependency would
// stall the whole pipeline.
const SendTimeout = 30 * time.Second

type Service struct {
	host     string
	port     int
	username string
	password string
	from     string
	baseURL  string

	// breaker stops hammering an SMTP server that is down (v1.0). While
	// it's open, sends fail immediately and the outbox retries them later
	// with backoff. nil disables it.
	breaker *breaker.Breaker
}

// WithBreaker guards every send with b.
func (s *Service) WithBreaker(b *breaker.Breaker) *Service {
	s.breaker = b
	return s
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
	ctx context.Context,
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

	send := func(ctx context.Context) error { return s.send(ctx, to, message) }
	var err error
	if s.breaker != nil {
		err = s.breaker.Do(ctx, send)
	} else {
		err = send(ctx)
	}
	if err != nil {
		return fmt.Errorf("failed to send activation email: %w", err)
	}

	return nil
}

// send is smtp.SendMail with a deadline: the same conversation (EHLO,
// STARTTLS when offered, AUTH, MAIL, RCPT, DATA, QUIT), but every step is
// bounded by the earlier of ctx's deadline and SendTimeout.
func (s *Service) send(ctx context.Context, to string, message []byte) error {
	ctx, cancel := context.WithTimeout(ctx, SendTimeout)
	defer cancel()

	address := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()

	// One deadline for the whole conversation: any read or write that
	// would block past it fails instead of hanging.
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	// Cancelling ctx (shutdown) also unblocks a read in progress.
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return err
	}
	defer c.Close()

	// Upgrade to TLS when the server offers it, as smtp.SendMail does, so
	// credentials are never sent in clear text.
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}

	// Local dev servers (MailHog, Mailtrap) may not need credentials.
	// PlainAuth refuses to send the password over an unencrypted
	// connection to anything but localhost.
	if s.username != "" && s.password != "" {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("smtp server does not support AUTH")
		}
		if err := c.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return err
		}
	}

	if err := c.Mail(s.from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(message); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
