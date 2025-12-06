package notifications

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"strings"
	"time"
)

// EmailConfig holds SMTP configuration for sending email notifications.
type EmailConfig struct {
	// Enabled determines if email notifications are active.
	Enabled bool

	// SMTPHost is the SMTP server hostname.
	SMTPHost string

	// SMTPPort is the SMTP server port (typically 25, 465, or 587).
	SMTPPort int

	// SMTPUser is the username for SMTP authentication (optional).
	SMTPUser string

	// SMTPPassword is the password for SMTP authentication (optional).
	SMTPPassword string

	// FromAddress is the sender email address.
	FromAddress string

	// FromName is the sender display name.
	FromName string

	// ToAddresses is the list of recipient email addresses.
	ToAddresses []string

	// UseTLS enables TLS/SSL connection (port 465).
	UseTLS bool

	// UseSTARTTLS enables STARTTLS upgrade (port 587).
	UseSTARTTLS bool

	// InsecureSkipVerify skips TLS certificate verification (for testing only).
	InsecureSkipVerify bool
}

// EmailSender handles sending email notifications.
type EmailSender struct {
	config EmailConfig
}

// NewEmailSender creates a new EmailSender with the given configuration.
func NewEmailSender(config EmailConfig) *EmailSender {
	return &EmailSender{config: config}
}

// IsEnabled returns true if email notifications are enabled and configured.
func (e *EmailSender) IsEnabled() bool {
	return e.config.Enabled &&
		e.config.SMTPHost != "" &&
		e.config.FromAddress != "" &&
		len(e.config.ToAddresses) > 0
}

// SendNotification sends an email notification.
func (e *EmailSender) SendNotification(n *Notification) error {
	if !e.IsEnabled() {
		return nil
	}

	subject := e.buildSubject(n)
	htmlBody, err := e.buildHTMLBody(n)
	if err != nil {
		return fmt.Errorf("building email body: %w", err)
	}

	textBody := e.buildTextBody(n)

	return e.send(subject, htmlBody, textBody)
}

// buildSubject creates the email subject line based on notification severity and title.
func (e *EmailSender) buildSubject(n *Notification) string {
	prefix := ""
	switch n.Severity {
	case SeverityError:
		prefix = "[ERROR] "
	case SeverityCritical:
		prefix = "[CRITICAL] "
	case SeverityWarning:
		prefix = "[WARNING] "
	default:
		prefix = "[INFO] "
	}
	return fmt.Sprintf("%sCaddyshack: %s", prefix, n.Title)
}

// buildTextBody creates a plain text version of the notification.
func (e *EmailSender) buildTextBody(n *Notification) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Notification from Caddyshack\n"))
	sb.WriteString(fmt.Sprintf("============================\n\n"))
	sb.WriteString(fmt.Sprintf("Type: %s\n", n.Type))
	sb.WriteString(fmt.Sprintf("Severity: %s\n", n.Severity))
	sb.WriteString(fmt.Sprintf("Title: %s\n", n.Title))
	sb.WriteString(fmt.Sprintf("Time: %s\n\n", n.CreatedAt.Format(time.RFC1123)))
	sb.WriteString(fmt.Sprintf("Message:\n%s\n", n.Message))
	if n.Data != "" {
		sb.WriteString(fmt.Sprintf("\nAdditional Data:\n%s\n", n.Data))
	}
	return sb.String()
}

// emailTemplateHTML is the HTML email template for notifications.
const emailTemplateHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Subject}}</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
        }
        .header {
            background-color: {{.SeverityColor}};
            color: white;
            padding: 20px;
            border-radius: 8px 8px 0 0;
        }
        .header h1 {
            margin: 0;
            font-size: 18px;
        }
        .severity-badge {
            display: inline-block;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: bold;
            text-transform: uppercase;
            background-color: rgba(255,255,255,0.2);
            margin-bottom: 10px;
        }
        .content {
            background-color: #f9fafb;
            padding: 20px;
            border: 1px solid #e5e7eb;
            border-top: none;
            border-radius: 0 0 8px 8px;
        }
        .message {
            background-color: white;
            padding: 15px;
            border-radius: 4px;
            border: 1px solid #e5e7eb;
            margin: 15px 0;
        }
        .meta {
            font-size: 12px;
            color: #6b7280;
            margin-top: 15px;
        }
        .footer {
            text-align: center;
            padding: 20px;
            font-size: 12px;
            color: #9ca3af;
        }
        .button {
            display: inline-block;
            background-color: #3b82f6;
            color: white;
            padding: 10px 20px;
            border-radius: 6px;
            text-decoration: none;
            margin-top: 15px;
        }
    </style>
</head>
<body>
    <div class="header">
        <div class="severity-badge">{{.SeverityLabel}}</div>
        <h1>{{.Title}}</h1>
    </div>
    <div class="content">
        <div class="message">
            {{.Message}}
        </div>
        <div class="meta">
            <p><strong>Type:</strong> {{.TypeLabel}}</p>
            <p><strong>Time:</strong> {{.Time}}</p>
        </div>
    </div>
    <div class="footer">
        <p>This notification was sent by Caddyshack.</p>
    </div>
</body>
</html>`

// emailTemplateData holds data for rendering the email template.
type emailTemplateData struct {
	Subject       string
	Title         string
	Message       string
	SeverityLabel string
	SeverityColor string
	TypeLabel     string
	Time          string
}

// buildHTMLBody creates an HTML version of the notification.
func (e *EmailSender) buildHTMLBody(n *Notification) (string, error) {
	severityColor := "#6b7280" // gray for info
	severityLabel := "Info"
	switch n.Severity {
	case SeverityError:
		severityColor = "#dc2626" // red
		severityLabel = "Error"
	case SeverityCritical:
		severityColor = "#ea580c" // orange
		severityLabel = "Critical"
	case SeverityWarning:
		severityColor = "#ca8a04" // yellow
		severityLabel = "Warning"
	}

	typeLabel := string(n.Type)
	switch n.Type {
	case TypeCertExpiry:
		typeLabel = "Certificate Expiry"
	case TypeDomainExpiry:
		typeLabel = "Domain Expiry"
	case TypeConfigChange:
		typeLabel = "Configuration Change"
	case TypeCaddyReload:
		typeLabel = "Caddy Reload"
	case TypeContainerDown:
		typeLabel = "Container Down"
	case TypeSystem:
		typeLabel = "System"
	}

	data := emailTemplateData{
		Subject:       e.buildSubject(n),
		Title:         n.Title,
		Message:       n.Message,
		SeverityLabel: severityLabel,
		SeverityColor: severityColor,
		TypeLabel:     typeLabel,
		Time:          n.CreatedAt.Format("January 2, 2006 at 3:04 PM MST"),
	}

	tmpl, err := template.New("email").Parse(emailTemplateHTML)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// send sends an email with the given subject and body.
func (e *EmailSender) send(subject, htmlBody, textBody string) error {
	// Build message
	var msg bytes.Buffer

	// Headers
	fromHeader := e.config.FromAddress
	if e.config.FromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", e.config.FromName, e.config.FromAddress)
	}
	msg.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(e.config.ToAddresses, ", ")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")

	// Multipart message with both text and HTML
	boundary := "boundary-caddyshack-notification"
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%s\r\n", boundary))
	msg.WriteString("\r\n")

	// Plain text part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(textBody)
	msg.WriteString("\r\n")

	// HTML part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)
	msg.WriteString("\r\n")

	// End boundary
	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	addr := fmt.Sprintf("%s:%d", e.config.SMTPHost, e.config.SMTPPort)

	// Set up authentication if credentials provided
	var auth smtp.Auth
	if e.config.SMTPUser != "" && e.config.SMTPPassword != "" {
		auth = smtp.PlainAuth("", e.config.SMTPUser, e.config.SMTPPassword, e.config.SMTPHost)
	}

	// TLS configuration
	tlsConfig := &tls.Config{
		ServerName:         e.config.SMTPHost,
		InsecureSkipVerify: e.config.InsecureSkipVerify,
	}

	// Send based on connection type
	if e.config.UseTLS {
		// Direct TLS connection (port 465)
		return e.sendWithTLS(addr, auth, tlsConfig, msg.Bytes())
	} else if e.config.UseSTARTTLS {
		// STARTTLS upgrade (port 587)
		return e.sendWithSTARTTLS(addr, auth, tlsConfig, msg.Bytes())
	}

	// Plain SMTP (port 25) - not recommended for production
	return smtp.SendMail(addr, auth, e.config.FromAddress, e.config.ToAddresses, msg.Bytes())
}

// sendWithTLS sends email using direct TLS connection (port 465).
func (e *EmailSender) sendWithTLS(addr string, auth smtp.Auth, tlsConfig *tls.Config, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, e.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	defer client.Close()

	return e.sendWithClient(client, auth, msg)
}

// sendWithSTARTTLS sends email using STARTTLS upgrade (port 587).
func (e *EmailSender) sendWithSTARTTLS(addr string, auth smtp.Auth, tlsConfig *tls.Config, msg []byte) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("SMTP dial: %w", err)
	}
	defer client.Close()

	// Send EHLO
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("EHLO: %w", err)
	}

	// Start TLS
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}

	return e.sendWithClient(client, auth, msg)
}

// sendWithClient sends the email using an established SMTP client.
func (e *EmailSender) sendWithClient(client *smtp.Client, auth smtp.Auth, msg []byte) error {
	// Authenticate if auth is provided
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
	}

	// Set sender
	if err := client.Mail(e.config.FromAddress); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}

	// Set recipients
	for _, to := range e.config.ToAddresses {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", to, err)
		}
	}

	// Send data
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("writing message: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("closing data writer: %w", err)
	}

	return client.Quit()
}

// ShouldSendEmail determines if an email should be sent for a notification based on severity.
// By default, emails are sent for critical and error notifications.
func ShouldSendEmail(n *Notification, sendOnWarning bool) bool {
	switch n.Severity {
	case SeverityCritical, SeverityError:
		return true
	case SeverityWarning:
		return sendOnWarning
	default:
		return false
	}
}

// EmailNotifier wraps the notification service to send emails when notifications are created.
type EmailNotifier struct {
	*Service
	emailSender    *EmailSender
	sendOnWarning  bool
}

// NewEmailNotifier creates a notifier that sends emails for important notifications.
func NewEmailNotifier(service *Service, emailSender *EmailSender, sendOnWarning bool) *EmailNotifier {
	return &EmailNotifier{
		Service:       service,
		emailSender:   emailSender,
		sendOnWarning: sendOnWarning,
	}
}

// Create creates a notification and optionally sends an email for critical notifications.
func (n *EmailNotifier) Create(notificationType Type, severity Severity, title, message, data string) (*Notification, error) {
	notif, err := n.Service.Create(notificationType, severity, title, message, data)
	if err != nil {
		return nil, err
	}

	// Send email if enabled and severity warrants it
	if n.emailSender != nil && n.emailSender.IsEnabled() && ShouldSendEmail(notif, n.sendOnWarning) {
		if err := n.emailSender.SendNotification(notif); err != nil {
			// Log the error but don't fail the notification creation
			log.Printf("Failed to send email notification: %v", err)
		}
	}

	return notif, nil
}
