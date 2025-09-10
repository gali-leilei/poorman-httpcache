package staff

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html/template"

	"httpcache/pkg/dbsqlc"

	"github.com/jackc/pgx/v5"
	"github.com/resend/resend-go/v2"
)

// HTML template for the email body
//
//nolint:lll
const emailHTML = `
<h2>Your Magic Link</h2>
<p>Hello,</p>

your magic link is: <a href="{{.MagicLink}}">{{.MagicLink}}</a>

<p>Best regards,<br>The Team</p>
`

// EmailData is the data for the email body
type EmailData struct {
	MagicLink   string
	EmailDomain string
}

// SendMailFunc is the function to send the email
type SendMailFunc func(sendTo string, onetimeToken string) (string, error)

// AllowlistFunc is the function to check if the email is allowed
type AllowlistFunc func(ctx context.Context, email string) (int64, error)

// NewAllowlist creates a new allowlist function
func NewAllowlist(db *pgx.Conn) (AllowlistFunc, error) {

	queries := dbsqlc.New(db)
	if queries == nil {
		return nil, fmt.Errorf("dbsqlc.New(): nil")
	}

	allowList := func(ctx context.Context, email string) (int64, error) {
		user, err := queries.GetUserByEmail(ctx, email)
		if err != nil {
			return 0, fmt.Errorf("queries.GetUserByEmail(ctx, email): %w", err)
		}
		return user.ID, nil
	}
	return allowList, nil

}

func NewSendMail(resendAPIKey string, emailDomain string, hostDomain string) (SendMailFunc, error) {
	resendClient := resend.NewClient(resendAPIKey)
	if resendClient == nil {
		return nil, fmt.Errorf("resend.NewClient(): nil")
	}

	emailTmpl, tmplErr := template.New("email").Parse(emailHTML)
	if tmplErr != nil {
		return nil, fmt.Errorf("template.New('email').Parse(emailHTML): nil")
	}

	sendMail := func(sendTo string, onetimeToken string) (string, error) {
		magicLink := fmt.Sprintf("%s/auth?onetime_token=%s", hostDomain, onetimeToken)
		emailData := EmailData{
			MagicLink:   magicLink,
			EmailDomain: emailDomain,
		}

		var emailBodyBuffer bytes.Buffer
		if err := emailTmpl.Execute(&emailBodyBuffer, emailData); err != nil {
			return "", fmt.Errorf("emailTmpl.Execute: %w", err)
		}
		emailBody := emailBodyBuffer.String()

		params := &resend.SendEmailRequest{
			From:    fmt.Sprintf("API Keys <noreply@%s>", emailDomain),
			To:      []string{sendTo},
			Html:    emailBody,
			Subject: "Your Secure Login Link",
		}

		sent, err := resendClient.Emails.Send(params)
		if err != nil {
			return "", fmt.Errorf("resendClient.Emails.Send: %w", err)
		}
		return sent.Id, nil
	}
	return sendMail, nil
}

func generateToken() (string, error) {
	b := make([]byte, 64)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
