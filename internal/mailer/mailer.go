// Package mailer sends email over SMTP (copied from Pocket Files, with HTML
// kept next to attachments and a Check that dials and signs in without
// sending anything).
package mailer

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Config is SMTP configuration.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	FromName string
	TLS      string // none | starttls | tls
}

// Attachment is a file to attach.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Message is an outgoing email.
type Message struct {
	To          []string
	Subject     string
	Body        string // plain text
	HTML        string // optional; sent as multipart/alternative next to Body
	Attachments []Attachment
}

// Configured reports whether SMTP is usable.
func (c Config) Configured() bool { return c.Host != "" && c.From != "" }

// dial connects, upgrades to TLS as configured and signs in.
func dial(cfg Config) (*smtp.Client, error) {
	if !cfg.Configured() {
		return nil, fmt.Errorf("SMTP is not set up (Settings → Email)")
	}
	port := cfg.Port
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	var client *smtp.Client
	var err error
	switch cfg.TLS {
	case "tls", "ssl":
		conn, derr := tls.DialWithDialer(&net.Dialer{Timeout: 20 * time.Second}, "tcp", addr, &tls.Config{ServerName: cfg.Host})
		if derr != nil {
			return nil, derr
		}
		client, err = smtp.NewClient(conn, cfg.Host)
	default:
		conn, derr := net.DialTimeout("tcp", addr, 20*time.Second)
		if derr != nil {
			return nil, derr
		}
		client, err = smtp.NewClient(conn, cfg.Host)
		if err == nil && cfg.TLS == "starttls" {
			if ok, _ := client.Extension("STARTTLS"); ok {
				err = client.StartTLS(&tls.Config{ServerName: cfg.Host})
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if cfg.User != "" {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)); err != nil {
				client.Close()
				return nil, fmt.Errorf("signing in to the mail server failed: %w", err)
			}
		}
	}
	return client, nil
}

// Check connects and signs in without sending anything.
func Check(cfg Config) error {
	c, err := dial(cfg)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Quit()
}

// Send delivers the message.
func Send(cfg Config, m Message) error {
	if len(m.To) == 0 {
		return fmt.Errorf("no recipient address")
	}
	client, err := dial(cfg)
	if err != nil {
		return err
	}
	defer client.Close()
	from := cfg.From
	if cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", cfg.FromName), cfg.From)
	}
	if err := client.Mail(cfg.From); err != nil {
		return err
	}
	for _, r := range m.To {
		if err := client.Rcpt(r); err != nil {
			return fmt.Errorf("rcpt %s: %w", r, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(Build(from, m)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// Build is the raw message (exported for tests).
func Build(from string, m Message) []byte {
	var b bytes.Buffer
	stamp := time.Now().UnixNano()
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(m.To, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	alt := fmt.Sprintf("psa-%d", stamp)
	body := func() {
		if m.HTML == "" {
			fmt.Fprintf(&b, "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n", wrap64(m.Body))
			return
		}
		fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", alt)
		fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n", alt, wrap64(m.Body))
		fmt.Fprintf(&b, "--%s\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n", alt, wrap64(m.HTML))
		fmt.Fprintf(&b, "--%s--\r\n", alt)
	}
	if len(m.Attachments) == 0 {
		body()
		return b.Bytes()
	}
	mixed := fmt.Sprintf("psm-%d", stamp)
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", mixed)
	fmt.Fprintf(&b, "--%s\r\n", mixed)
	body()
	for _, a := range m.Attachments {
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		name := mime.QEncoding.Encode("utf-8", a.Filename)
		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; name=%q\r\nContent-Transfer-Encoding: base64\r\nContent-Disposition: attachment; filename=%q\r\n\r\n%s\r\n", mixed, ct, name, name, wrap64(string(a.Data)))
	}
	fmt.Fprintf(&b, "--%s--\r\n", mixed)
	return b.Bytes()
}

func wrap64(s string) string {
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	var b strings.Builder
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc)
	return b.String()
}
