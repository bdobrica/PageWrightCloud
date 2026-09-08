// Package mail sends reset links only through explicitly configured TLS SMTP.
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Sender struct {
	Address, Username, Password, From, ResetURL, Mode string
	tlsConfig                                         *tls.Config
}

func FromEnv(origins string) (*Sender, error) {
	s := &Sender{Address: os.Getenv("PAGEWRIGHT_SMTP_ADDRESS"), Username: os.Getenv("PAGEWRIGHT_SMTP_USERNAME"), Password: os.Getenv("PAGEWRIGHT_SMTP_PASSWORD"), From: os.Getenv("PAGEWRIGHT_SMTP_FROM"), ResetURL: os.Getenv("PAGEWRIGHT_RESET_URL"), Mode: os.Getenv("PAGEWRIGHT_SMTP_MODE")}
	if *s == (Sender{}) {
		return nil, nil
	}
	if s.Mode == "" {
		s.Mode = "starttls"
	}
	if err := s.Validate(origins); err != nil {
		return nil, err
	}
	return s, nil
}

func address(value string) bool {
	a, err := mail.ParseAddress(value)
	return err == nil && a.Address == value && !strings.ContainsAny(value, "\r\n") && len(value) <= 254
}

func (s *Sender) Validate(origins string) error {
	bad := errors.New("invalid SMTP/reset configuration; require address, sender, TLS mode and an allowed application reset URL")
	host, port, err := net.SplitHostPort(s.Address)
	number, portErr := strconv.Atoi(port)
	if err != nil || host == "" || strings.ContainsAny(host, " /\\\t\r\n@?#") || portErr != nil || number < 1 || number > 65535 || !address(s.From) || (s.Mode != "starttls" && s.Mode != "tls") || (s.Username == "") != (s.Password == "") {
		return bad
	}
	u, err := url.Parse(s.ResetURL)
	if err != nil || u.User != nil || u.Path != "/reset-password" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return bad
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")) {
		return bad
	}
	for _, origin := range strings.Split(origins, ",") {
		if strings.TrimSpace(origin) == u.Scheme+"://"+u.Host {
			return nil
		}
	}
	return bad
}

func (s *Sender) SendReset(ctx context.Context, recipient, token string) error {
	if s.Mode != "tls" && s.Mode != "starttls" {
		return errors.New("SMTP requires an explicit TLS mode")
	}
	if !address(recipient) {
		return errors.New("invalid reset recipient")
	}
	host, _, err := net.SplitHostPort(s.Address)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.Address)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { rawConn.Close() })
	defer stop()
	tlsConfig := s.tlsConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	}
	if s.Mode == "tls" {
		secure := tls.Client(conn, tlsConfig)
		if err := secure.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if s.Mode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP requires STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if s.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.Username, s.Password, host)); err != nil {
			return err
		}
	}
	if err := client.Mail(s.From); err != nil {
		return err
	}
	if err := client.Rcpt(recipient); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	link := s.ResetURL + "#token=" + url.QueryEscape(token)
	_, err = fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: Reset your PageWright password\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nUse this link within one hour to reset your password:\r\n%s\r\n\r\nIf you did not request this, ignore this message.\r\n", s.From, recipient, link)
	if err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	// DATA acknowledgement is the delivery boundary; a lost QUIT is not failure.
	_ = client.Quit()
	return nil
}
