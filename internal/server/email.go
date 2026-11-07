package server

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/mailer"
	"github.com/abjelosevic88/pocket-homelab-league/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- email ----
//
// Players are often not on this box, so League emails them: their sign-in
// link, and (when the group wants it) plans and reminders. The emails go
// out the way the organiser chose (Settings → Email): through Pocket Mail
// (a Core link with mail:send, or Direct), from their own mailbox; through
// their own SMTP server; or not at all — then they share the links by
// WhatsApp, SMS or by copying them.

// outMail is one email League sends for a person.
type outMail struct {
	To      []string
	ToName  string
	Subject string
	HTML    string
	Text    string
}

// errEmailOff: the person chose not to send emails.
var errEmailOff = errors.New("email is off (Settings → Email): share the link by WhatsApp or copy it")

// emailVia is how a person's emails go out: mail | smtp | none.
func (s *Server) emailVia(st store.Settings) string {
	if st.EmailVia != "" {
		return st.EmailVia
	}
	if s.core.Configured() {
		return "mail"
	}
	return "none"
}

// sendMail sends m for person uid the way they chose (Pocket Mail, SMTP).
func (s *Server) sendMail(ctx context.Context, uid int64, m outMail) error {
	if len(m.To) == 0 || strings.TrimSpace(m.To[0]) == "" {
		return errors.New("no email address")
	}
	if s.cfg.DemoData {
		s.log.Debug("demo: email not sent", "to", m.To, "subject", m.Subject)
		return nil
	}
	st, err := s.store.GetSettings(ctx, uid)
	if err != nil {
		return err
	}
	switch s.emailVia(st) {
	case "mail":
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		conn, err := s.clients(ctx, uid).conn(ctx, appMail, scopeMailSend)
		if err != nil {
			return err
		}
		pm := pocket.Mail{From: st.MailFrom, Subject: m.Subject, BodyHTML: m.HTML, BodyText: m.Text}
		for i, to := range m.To {
			a := pocket.MailAddress{Addr: to}
			if i == 0 {
				a.Name = m.ToName
			}
			pm.To = append(pm.To, a)
		}
		if err := conn.client.SendMail(ctx, pm); err != nil {
			return fmt.Errorf("Pocket Mail: %w", err)
		}
		return nil
	case "smtp":
		return mailer.Send(s.smtpConfig(ctx, uid, st), mailer.Message{To: m.To, Subject: m.Subject, Body: m.Text, HTML: m.HTML})
	}
	return errEmailOff
}

// smtpConfig is a person's own SMTP server.
func (s *Server) smtpConfig(ctx context.Context, uid int64, st store.Settings) mailer.Config {
	name := ""
	if u, err := s.store.GetUser(ctx, uid); err == nil {
		name = u.Name
	}
	return mailer.Config{Host: st.SMTPHost, Port: st.SMTPPort, User: st.SMTPUser, Password: st.SMTPPassword,
		From: firstNonEmpty(st.SMTPFrom, st.SMTPUser), FromName: firstNonEmpty(st.SMTPFromName, name), TLS: st.SMTPTLS}
}

// ---- the emails ----

// mailBody is the content of one email; render turns it into HTML + text.
type mailBody struct {
	Heading string
	Lines   []string
	Button  string
	URL     string
	After   []string
	Footer  string
}

const mailAccent = "#2f9e44"

func (b mailBody) render() (htmlOut, text string) {
	var h, t strings.Builder
	esc := html.EscapeString
	para := func(s string) string { return strings.ReplaceAll(esc(s), "\n", "<br>") }
	h.WriteString(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark"></head>`)
	h.WriteString(`<body style="margin:0;padding:0;background:#f5f6f8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2433">`)
	h.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f5f6f8;padding:24px 12px"><tr><td align="center">`)
	h.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border-radius:14px;border:1px solid #e3e6ee;padding:28px">`)
	h.WriteString(`<tr><td style="font-size:20px;line-height:1.3;font-weight:650;padding-bottom:14px">` + esc(b.Heading) + `</td></tr>`)
	t.WriteString(b.Heading + "\n\n")
	for _, l := range b.Lines {
		h.WriteString(`<tr><td style="font-size:15px;line-height:1.55;padding-bottom:12px">` + para(l) + `</td></tr>`)
		t.WriteString(l + "\n\n")
	}
	if b.Button != "" && b.URL != "" {
		h.WriteString(`<tr><td align="center" style="padding:8px 0 18px"><a href="` + esc(b.URL) + `" style="display:inline-block;background:` + mailAccent + `;color:#ffffff;text-decoration:none;font-weight:600;font-size:16px;padding:12px 26px;border-radius:10px">` + esc(b.Button) + `</a></td></tr>`)
		h.WriteString(`<tr><td style="font-size:12px;line-height:1.5;color:#6b7183;padding-bottom:14px;word-break:break-all">Or open this address: <a href="` + esc(b.URL) + `" style="color:` + mailAccent + `">` + esc(b.URL) + `</a></td></tr>`)
		t.WriteString(b.Button + ": " + b.URL + "\n\n")
	}
	for _, l := range b.After {
		h.WriteString(`<tr><td style="font-size:14px;line-height:1.55;color:#4a5064;padding-bottom:10px">` + para(l) + `</td></tr>`)
		t.WriteString(l + "\n\n")
	}
	if b.Footer != "" {
		h.WriteString(`<tr><td style="font-size:12px;line-height:1.5;color:#8a90a2;border-top:1px solid #eceef3;padding-top:14px">` + para(b.Footer) + `</td></tr>`)
		t.WriteString("—\n" + b.Footer + "\n")
	}
	h.WriteString(`</table></td></tr></table></body></html>`)
	return h.String(), t.String()
}

func sportLine(g *store.Group) string {
	sp := league.SportByKey(g.Sport)
	return strings.TrimSpace(sp.Emoji + " " + league.DisplayName(g.Sport, g.SportName))
}

// inviteMail brings a player in: who added them to what, and their link.
func (s *Server) inviteMail(ctx context.Context, by, u *store.User, g *store.Group, link string) outMail {
	who := firstNonEmpty(by.Name, by.Email)
	b := mailBody{
		Heading: who + " added you to " + g.Name,
		Lines: []string{
			fmt.Sprintf("Hi %s! %s plays %s with you and keeps the group in Pocket League: when the next game is, who is coming, the scores and the table, and a chat for the group.",
				firstName(u.Name), who, strings.ToLower(league.DisplayName(g.Sport, g.SportName))),
			"Tap the button to come in — no password, nothing to install. It works on any phone or computer.",
		},
		Button: "Join " + g.Name, URL: link,
		After:  []string{"The button works once, for 14 days. On a new phone later, ask for a new link on the sign-in page with this email address."},
		Footer: "Sent with Pocket League for " + who + ". If you don't know them, ignore this email — nothing happens unless you tap the button.",
	}
	h, t := b.render()
	return outMail{To: []string{u.Email}, ToName: u.Name, Subject: who + " added you to " + g.Name + " (" + sportLine(g) + ")", HTML: h, Text: t}
}

// newLinkMail answers "send me a new link".
func (s *Server) newLinkMail(ctx context.Context, by, u *store.User, g *store.Group, link string) outMail {
	b := mailBody{
		Heading: "Your new link for Pocket League",
		Lines:   []string{"Hi " + firstName(u.Name) + "! Here is a new way in to " + g.Name + " and your other groups."},
		Button:  "Open Pocket League", URL: link,
		After:  []string{"It works once, for 14 days. If you did not ask for it, you can ignore this email."},
		Footer: "Sent with Pocket League for " + firstNonEmpty(by.Name, by.Email) + ".",
	}
	h, t := b.render()
	return outMail{To: []string{u.Email}, ToName: u.Name, Subject: "Your new link for " + g.Name, HTML: h, Text: t}
}

// noteMail is a notification (a game planned, a reminder) as an email.
func (s *Server) noteMail(u *store.User, g *store.Group, n note) outMail {
	b := mailBody{Heading: n.title, Lines: strings.Split(n.body, "\n"), Button: firstNonEmpty(n.button, "Open in Pocket League"), URL: n.link,
		Footer: g.Name + " · " + sportLine(g) + ". You get these emails because you are in this group; turn them off in Pocket League → Settings → Notifications."}
	h, t := b.render()
	return outMail{To: []string{u.Email}, ToName: u.Name, Subject: n.title, HTML: h, Text: t}
}

// ---- POST /email/test ----

func (s *Server) handleEmailTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	if u.Email == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "your account has no email address"})
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	b := mailBody{Heading: "Email from Pocket League works",
		Lines:  []string{"This is a test. Sign-in links for your players, plans and reminders will go out like this one."},
		Button: "Open Pocket League", URL: s.publicBase(r), Footer: "Pocket League"}
	h, t := b.render()
	err := s.mailer(ctx, u.ID, outMail{To: []string{u.Email}, ToName: u.Name, Subject: "Pocket League: test email", HTML: h, Text: t})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	how := map[string]string{"mail": " through Pocket Mail", "smtp": " through your mail server"}[s.emailVia(st)]
	if s.cfg.DemoData {
		how = " (demo mode: nothing really leaves the app)"
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Sent to " + u.Email + how})
}
