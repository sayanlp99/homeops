package api

import (
	"fmt"
	"html"
	"strings"

	"homeops/internal/mail"
)

var roleDescriptions = map[string]string{
	"owner":  "an owner",
	"admin":  "an admin",
	"member": "a member",
	"viewer": "a viewer (read-only)",
}

// emailLayout renders a plain, client-safe HTML email: one column, inline styles.
func emailLayout(heading string, paragraphs []string, buttonLabel, buttonURL, footer string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><body style="margin:0;padding:24px 12px;background:#f4f5f7;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2328">`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center">`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:480px;background:#ffffff;border:1px solid #e3e5e8;border-radius:8px">`)
	b.WriteString(`<tr><td style="padding:20px 24px;border-bottom:1px solid #e3e5e8;font-size:14px;font-weight:600;color:#2563eb">HomeOps</td></tr>`)
	fmt.Fprintf(&b, `<tr><td style="padding:24px"><h1 style="margin:0 0 12px;font-size:18px;line-height:1.35">%s</h1>`, html.EscapeString(heading))
	for _, p := range paragraphs {
		fmt.Fprintf(&b, `<p style="margin:0 0 12px;font-size:14px;line-height:1.55;color:#424a53">%s</p>`, html.EscapeString(p))
	}
	if buttonURL != "" {
		fmt.Fprintf(&b, `<p style="margin:20px 0"><a href="%s" style="display:inline-block;background:#2563eb;color:#ffffff;text-decoration:none;font-size:14px;font-weight:600;padding:10px 18px;border-radius:6px">%s</a></p>`,
			html.EscapeString(buttonURL), html.EscapeString(buttonLabel))
		fmt.Fprintf(&b, `<p style="margin:0;font-size:12px;line-height:1.5;color:#6e7781">Or paste this link into your browser:<br><a href="%[1]s" style="color:#2563eb;word-break:break-all">%[1]s</a></p>`,
			html.EscapeString(buttonURL))
	}
	b.WriteString(`</td></tr>`)
	fmt.Fprintf(&b, `<tr><td style="padding:14px 24px;border-top:1px solid #e3e5e8;font-size:12px;line-height:1.5;color:#6e7781">%s</td></tr>`, html.EscapeString(footer))
	b.WriteString(`</table></td></tr></table></body></html>`)
	return b.String()
}

func inviteEmail(to, inviter, household, role, url string) mail.Message {
	heading := fmt.Sprintf("Join %s on HomeOps", household)
	paras := []string{
		fmt.Sprintf("%s invited you to %s as %s.", inviter, household, roleDescriptions[role]),
		"HomeOps is where the household plans meals, shares chores in weekly sprints and keeps the budget.",
	}
	footer := "This invite works once and expires in 7 days. If you weren't expecting it, you can ignore this email."
	return mail.Message{
		To:      to,
		Subject: fmt.Sprintf("%s invited you to %s", inviter, household),
		Text:    fmt.Sprintf("%s\n\n%s\n\nAccept the invite: %s\n\n%s\n", paras[0], paras[1], url, footer),
		HTML:    emailLayout(heading, paras, "Accept invite", url, footer),
	}
}

func addedEmail(to, admin, household, url string) mail.Message {
	p := fmt.Sprintf("%s created %s on HomeOps and made you its owner. Sign in and switch to it from the household menu.", admin, household)
	footer := "You're receiving this because an account with this email exists on HomeOps."
	return mail.Message{
		To:      to,
		Subject: "You're the owner of " + household,
		Text:    fmt.Sprintf("%s\n\nOpen HomeOps: %s\n", p, url),
		HTML:    emailLayout("New household: "+household, []string{p}, "Open HomeOps", url, footer),
	}
}

func resetEmail(to, name, url string) mail.Message {
	p := []string{
		fmt.Sprintf("Hi %s, someone (hopefully you) asked to reset your HomeOps password.", name),
		"Choose a new password with the button below. You'll be signed out of your other devices.",
	}
	footer := "The link expires in 1 hour and works once. If you didn't ask for this, ignore this email: your password stays the same."
	return mail.Message{
		To:      to,
		Subject: "Reset your HomeOps password",
		Text:    fmt.Sprintf("%s\n%s\n\nReset password: %s\n\n%s\n", p[0], p[1], url, footer),
		HTML:    emailLayout("Reset your password", p, "Choose a new password", url, footer),
	}
}
