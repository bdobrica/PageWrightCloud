package mail

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConfiguration(t *testing.T) {
	s := Sender{Address: "smtp.example.test:587", From: "reset@example.test", ResetURL: "https://app.example.test/reset-password", Mode: "starttls"}
	if err := s.Validate("https://app.example.test"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"https://smtp.example.test:587", "smtp.example.test:0", "smtp.example.test:65536", "smtp.example.test:smtp"} {
		bad := s
		bad.Address = value
		if bad.Validate("https://app.example.test") == nil {
			t.Fatal("invalid SMTP address accepted")
		}
	}
	for _, url := range []string{"http://app.example.test/reset-password", "https://evil.test/reset-password", "https://app.example.test/reset-password?token=x", "https://app.example.test/reset-password#x", "https://user@app.example.test/reset-password"} {
		bad := s
		bad.ResetURL = url
		if bad.Validate("https://app.example.test") == nil {
			t.Fatal("unsafe reset URL accepted")
		}
	}
	if address("victim@example.test\r\nBcc: thief@example.test") {
		t.Fatal("header injection accepted")
	}
}

func TestRejectUntrustedCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	s := Sender{Address: strings.TrimPrefix(server.URL, "https://"), From: "reset@example.test", Mode: "tls", ResetURL: "https://app.example.test/reset-password"}
	if err := s.SendReset(context.Background(), "user@example.test", "secret"); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
}

// Use an actual SMTP dialogue with a verified private test CA. Production never
// disables verification and never falls back to plaintext when TLS is absent.
func TestTLSResetDelivery(t *testing.T) {
	for _, mode := range []string{"tls", "starttls"} {
		t.Run(mode, func(t *testing.T) { testResetDelivery(t, mode) })
	}
}

func testResetDelivery(t *testing.T, mode string) {
	s, received := smtpFixture(t, mode)
	if err := s.SendReset(context.Background(), "user@example.test", "secret-token"); err != nil {
		t.Fatal(err)
	}
	message := <-received
	for _, want := range []string{"To: user@example.test", "From: reset@example.test", "https://app.example.test/reset-password#token=secret-token"} {
		if !strings.Contains(message, want) {
			t.Fatalf("missing mail field: %s", want)
		}
	}
	if strings.Contains(message, "test-only") {
		t.Fatal("SMTP credential in message")
	}
}

func smtpFixture(t *testing.T, mode string) (Sender, <-chan string) {
	t.Helper()
	certServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(certServer.Close)
	roots := x509.NewCertPool()
	roots.AddCert(certServer.Certificate())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			received <- "accept failed"
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		if mode == "tls" {
			conn = tls.Server(conn, certServer.TLS)
		}
		reader := bufio.NewReader(conn)
		fmt.Fprint(conn, "220 localhost ESMTP\r\n")
		var message strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				received <- "SMTP failed"
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				fmt.Fprint(conn, "250-localhost\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n")
			case strings.HasPrefix(line, "STARTTLS"):
				fmt.Fprint(conn, "220 begin TLS\r\n")
				conn = tls.Server(conn, certServer.TLS)
				reader = bufio.NewReader(conn)
			case strings.HasPrefix(line, "AUTH"):
				fmt.Fprint(conn, "235 authenticated\r\n")
			case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
				fmt.Fprint(conn, "250 OK\r\n")
			case strings.HasPrefix(line, "DATA"):
				fmt.Fprint(conn, "354 continue\r\n")
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						received <- "DATA failed"
						return
					}
					if line == ".\r\n" {
						break
					}
					message.WriteString(line)
				}
				fmt.Fprint(conn, "250 queued\r\n")
			case strings.HasPrefix(line, "QUIT"):
				fmt.Fprint(conn, "221 bye\r\n")
				received <- message.String()
				return
			default:
				received <- "unexpected command"
				return
			}
		}
	}()
	s := Sender{Address: listener.Addr().String(), From: "reset@example.test", Username: "test-user", Password: "test-only", ResetURL: "https://app.example.test/reset-password", Mode: mode, tlsConfig: &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}}
	return s, received
}

func TestNoPlaintextFallback(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		fmt.Fprint(conn, "220 local SMTP\r\n")
		reader := bufio.NewReader(conn)
		reader.ReadString('\n')
		fmt.Fprint(conn, "250 local\r\n")
	}()
	s := Sender{Address: listener.Addr().String(), From: "reset@example.test", ResetURL: "https://app.example.test/reset-password", Mode: "starttls"}
	if err := s.SendReset(context.Background(), "user@example.test", "secret"); err == nil {
		t.Fatal("plaintext accepted")
	}
	<-done
}
