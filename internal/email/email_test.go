package email

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/breaker"
)

// fakeSMTP speaks just enough SMTP to accept one message per connection.
type fakeSMTP struct {
	ln   net.Listener
	mu   sync.Mutex
	got  []string
	hang bool // accept the connection, then never answer
}

func startSMTP(t *testing.T, hang bool) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, hang: hang}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(conn net.Conn) {
	defer conn.Close()
	if f.hang {
		time.Sleep(time.Minute) // the bug v1.0 fixes: a silent server
		return
	}
	r := bufio.NewReader(conn)
	say := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	say("220 fake ESMTP")
	var data strings.Builder
	inData := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if inData {
			if line == ".\r\n" {
				inData = false
				f.mu.Lock()
				f.got = append(f.got, data.String())
				f.mu.Unlock()
				say("250 queued")
				continue
			}
			data.WriteString(line)
			continue
		}
		switch cmd := strings.ToUpper(strings.Fields(line)[0]); cmd {
		case "EHLO", "HELO":
			say("250 fake")
		case "MAIL", "RCPT":
			say("250 ok")
		case "DATA":
			inData = true
			say("354 go ahead")
		case "QUIT":
			say("221 bye")
			return
		default:
			say("502 not implemented")
		}
	}
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func service(port int) *Service {
	return NewService("127.0.0.1", port, "", "", "no-reply@auction.local", "http://localhost:4000")
}

func TestSendDeliversMessage(t *testing.T) {
	f := startSMTP(t, false)
	if err := service(f.port()).SendActivationEmail(context.Background(), "alice@example.com", "tok123"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.got) != 1 || !strings.Contains(f.got[0], "/v1/users/activate?token=tok123") || !strings.Contains(f.got[0], "To: alice@example.com") {
		t.Fatalf("got %q", f.got)
	}
}

// A server that accepts the connection and then says nothing must not hang
// the outbox worker: the send fails when the caller's deadline passes.
func TestHungServerTimesOut(t *testing.T) {
	f := startSMTP(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := service(f.port()).SendActivationEmail(ctx, "alice@example.com", "tok")
	if err == nil {
		t.Fatal("expected an error from a silent server")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("send blocked for %s; deadline not enforced", took)
	}
}

// When SMTP is down, the breaker opens and later sends fail without even
// dialling, so the outbox backs off instead of hammering the server.
func TestBreakerStopsDiallingADeadServer(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // nothing listens: connection refused

	b := breaker.New("smtp-test", breaker.Options{Threshold: 2, Cooldown: time.Hour})
	s := service(port).WithBreaker(b)
	for range 2 {
		_ = s.SendActivationEmail(context.Background(), "a@example.com", "t")
	}
	err := s.SendActivationEmail(context.Background(), "a@example.com", "t")
	if !errors.Is(err, breaker.ErrOpen) {
		t.Fatalf("third send: %v, want ErrOpen", err)
	}
}
