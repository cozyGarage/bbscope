package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"time"
)

// EmailNotifier sends notifications via email
type EmailNotifier struct {
	config *EmailConfig
}

// NewEmailNotifier creates a new email notifier
func NewEmailNotifier(cfg *EmailConfig) *EmailNotifier {
	return &EmailNotifier{
		config: cfg,
	}
}

// Name returns the notifier name
func (e *EmailNotifier) Name() string {
	return "email"
}

// Send sends a notification via email
func (e *EmailNotifier) Send(ctx context.Context, event ChangeEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.config == nil || len(e.config.To) == 0 {
		return fmt.Errorf("email notifier: no recipients configured")
	}
	if !shouldSend(e.config.Events, event.Type) {
		return nil
	}

	subject := formatEmailSubject(event)
	body := formatEmailBody(event)

	message := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-version: 1.0;\r\n"+
		"Content-Type: text/html; charset=\"UTF-8\";\r\n"+
		"\r\n"+
		"%s\r\n",
		sanitizeHeaderField(e.config.From),
		sanitizeHeaderField(e.config.To[0]),
		subject,
		body,
	)

	// Connect to SMTP server
	addr := fmt.Sprintf("%s:%d", e.config.SMTPHost, e.config.SMTPPort)

	// Use TLS if port is 465, otherwise use STARTTLS
	if e.config.SMTPPort == 465 {
		return e.sendWithTLS(addr, message)
	}
	return e.sendWithSTARTTLS(addr, message)
}

// sendWithSTARTTLS sends email using STARTTLS (ports 587, 25)
func (e *EmailNotifier) sendWithSTARTTLS(addr, message string) error {
	// Create auth if credentials provided
	var auth smtp.Auth
	if e.config.Username != "" && e.config.Password != "" {
		auth = smtp.PlainAuth("", e.config.Username, e.config.Password, e.config.SMTPHost)
	}

	// Send email
	return smtp.SendMail(addr, auth, e.config.From, e.config.To, []byte(message))
}

// sendWithTLS sends email using direct TLS connection (port 465)
func (e *EmailNotifier) sendWithTLS(addr, message string) error {
	// Create TLS config
	tlsConfig := &tls.Config{
		ServerName: e.config.SMTPHost,
		MinVersion: tls.VersionTLS12,
	}

	// Connect with TLS
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS dial failed: %w", err)
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, e.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("SMTP client creation failed: %w", err)
	}
	defer func() { _ = client.Quit() }()

	// Authenticate if credentials provided
	if e.config.Username != "" && e.config.Password != "" {
		auth := smtp.PlainAuth("", e.config.Username, e.config.Password, e.config.SMTPHost)
		err = client.Auth(auth)
		if err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	// Set sender and recipients
	err = client.Mail(e.config.From)
	if err != nil {
		return fmt.Errorf("MAIL command failed: %w", err)
	}

	for _, to := range e.config.To {
		err = client.Rcpt(to)
		if err != nil {
			return fmt.Errorf("RCPT command failed for %s: %w", to, err)
		}
	}

	// Send message
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA command failed: %w", err)
	}
	if _, err := wc.Write([]byte(message)); err != nil {
		_ = wc.Close()
		return fmt.Errorf("failed to write message: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to finish message: %w", err)
	}

	return nil
}

// formatEmailSubject creates an email subject from a change event
func formatEmailSubject(event ChangeEvent) string {
	emoji := getEmailEmoji(event.Type)
	return fmt.Sprintf("%s bbscope: %s scope change on %s",
		emoji, sanitizeHeaderField(event.Type), sanitizeHeaderField(event.Platform))
}

// formatEmailBody creates an HTML email body from a change event
func formatEmailBody(event ChangeEvent) string {
	color := getEmailColor(event.Type)
	emoji := getEmailEmoji(event.Type)

	var scopeBadge string
	if event.InScope {
		scopeBadge = `<span style="background-color: #28a745; color: white; padding: 2px 8px; border-radius: 3px;">✅ In-Scope</span>`
	} else {
		scopeBadge = `<span style="background-color: #dc3545; color: white; padding: 2px 8px; border-radius: 3px;">❌ Out-of-Scope</span>`
	}

	var bountyBadge string
	if event.IsBBP {
		bountyBadge = `<span style="background-color: #ffc107; color: black; padding: 2px 8px; border-radius: 3px;">💰 Bounty</span>`
	} else {
		bountyBadge = `<span style="background-color: #6c757d; color: white; padding: 2px 8px; border-radius: 3px;">No Bounty</span>`
	}

	platform := escapeHTMLText(event.Platform)
	changeType := escapeHTMLText(event.Type)
	category := escapeHTMLText(event.Category)
	target := escapeHTMLText(event.Target)
	handle := escapeHTMLText(event.ProgramHandle)
	programHref := safeHTTPURL(event.ProgramURL)
	programHTML := handle
	if programHref != "" {
		programHTML = fmt.Sprintf(`<a href="%s">%s</a>`, escapeHTMLText(programHref), handle)
	} else if event.ProgramURL != "" {
		programHTML = escapeHTMLText(event.ProgramURL)
	}

	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background-color: %s; color: white; padding: 15px; border-radius: 5px; }
        .content { background-color: #f8f9fa; padding: 20px; margin: 20px 0; border-radius: 5px; }
        .field { margin: 10px 0; }
        .label { font-weight: bold; color: #666; }
        .value { color: #333; }
        .footer { color: #999; font-size: 12px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>%s Scope Change on %s</h2>
        </div>
        <div class="content">
            <div class="field">
                <span class="label">Type:</span>
                <span class="value">%s</span>
            </div>
            <div class="field">
                <span class="label">Category:</span>
                <span class="value">%s</span>
            </div>
            <div class="field">
                <span class="label">Target:</span>
                <span class="value"><code>%s</code></span>
            </div>
            <div class="field">
                <span class="label">Program:</span>
                <span class="value">%s</span>
            </div>
            <div class="field">
                <span class="label">Status:</span>
                <span class="value">%s %s</span>
            </div>
            <div class="field">
                <span class="label">Time:</span>
                <span class="value">%s</span>
            </div>
        </div>
        <div class="footer">
            This notification was sent by bbscope
        </div>
    </div>
</body>
</html>
`,
		color,
		emoji,
		platform,
		changeType,
		category,
		target,
		programHTML,
		scopeBadge,
		bountyBadge,
		escapeHTMLText(event.OccurredAt.Format(time.RFC1123)),
	)
}

// getEmailColor returns the color for a change type
func getEmailColor(changeType string) string {
	switch changeType {
	case "added":
		return "#28a745" // green
	case "removed":
		return "#dc3545" // red
	case "updated":
		return "#ffc107" // yellow
	default:
		return "#6c757d" // gray
	}
}

// getEmailEmoji returns the emoji for a change type
func getEmailEmoji(changeType string) string {
	switch changeType {
	case "added":
		return "🆕"
	case "removed":
		return "🗑️"
	case "updated":
		return "🔄"
	default:
		return "📝"
	}
}
