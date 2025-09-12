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

// HTML template for the email body
//
//nolint:lll
const instructionHTML = `
<h2>Your API Key</h2>
<p>Hello,</p>
<p>Your API key is: <strong>{{.APIKey}}</strong></p>
<p>Please keep this key secure and do not share it with others.</p>

<h3>Bash Examples</h3>
<p>Use your API key to access our cached proxy services:</p>

<h4>Jina AI</h4>
<pre style="background-color: #f6f8fa; padding: 16px; border-radius: 6px; border: 1px solid #d1d9e0; overflow-x: auto; font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace; font-size: 85%;">
curl --location "{{.ServiceDomain}}/jina/https://www.example.com" \
  --header "Authorization: Bearer {{.APIKey}}"</pre>

<h4>Serper</h4>
<pre style="background-color: #f6f8fa; padding: 16px; border-radius: 6px; border: 1px solid #d1d9e0; overflow-x: auto; font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace; font-size: 85%;">
curl --location "{{.ServiceDomain}}/serper/search" \
  --header "X-API-KEY: {{.APIKey}}" \
  --header "Content-Type: application/json" \
  --data '{"q": "your search query"}'</pre>

<h3>Python Example:</h3>
<p>Here are Python examples for using the endpoints:</p>

<h4>Jina AI</h4>
<pre style="background-color: #f6f8fa; padding: 16px; border-radius: 6px; border: 1px solid #d1d9e0; overflow-x: auto; font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace; font-size: 85%;">
import requests

url = "{{.ServiceDomain}}/jina/https://www.example.com"
headers = {
    "Authorization": "Bearer {{.APIKey}}"
}

response = requests.get(url, headers=headers)
print(response.json())
</pre>

<h4>Serper</h4>
<pre style="background-color: #f6f8fa; padding: 16px; border-radius: 6px; border: 1px solid #d1d9e0; overflow-x: auto; font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace; font-size: 85%;">
import requests
import json

url = "{{.ServiceDomain}}/serper/search"
headers = {
    "X-API-KEY": "{{.APIKey}}",
    "Content-Type": "application/json"
}
data = {
    "q": "your search query"
}

response = requests.post(url, headers=headers, json=data)
print(response.json())
</pre>

<h4>With Miroflow</h4>
<p> Coming soon (PR awaiting test and review). Update project .env file or config.yaml file: </p>
<pre style="background-color: #f6f8fa; padding: 16px; border-radius: 6px; border: 1px solid #d1d9e0; overflow-x: auto; font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace; font-size: 85%;">
// in .env file
JINA_BASE_URL={{.ServiceDomain}}/jina/
JINA_API_KEY={{.APIKey}}
SERPER_BASE_URL={{.ServiceDomain}}/serper/
SERPER_API_KEY={{.APIKey}}

// in config.yaml file
env:
	JINA_BASE_URL: {{.ServiceDomain}}/jina/
	JINA_API_KEY: {{.APIKey}}
	SERPER_BASE_URL: {{.ServiceDomain}}/serper/
	SERPER_API_KEY: {{.APIKey}}
</pre>

<p>Best regards,<br>The Team</p>
`

// EmailData is the data for the email body
type EmailData struct {
	MagicLink   string
	EmailDomain string
}

// InstructionData is the data for the instruction email
type InstructionData struct {
	APIKey        string
	ServiceDomain string
}

// SendMailFunc is the function to send the email
type SendMailFunc func(sendTo string, onetimeToken string) (string, error)

// SendInstructionFunc is the function to send the instruction email
type SendInstructionFunc func(sendTo string, apiKey string) (string, error)

// AllowlistFunc is the function to check if the email is allowed
type AllowlistFunc func(ctx context.Context, email string) (int64, error)

// IDToKeyFunc is the function to get the API key for a user
type IDToKeyFunc func(ctx context.Context, userID int64) (string, error)

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

// NewIDToKey creates a new IDToKey function
func NewIDToKey(db *pgx.Conn) (IDToKeyFunc, error) {
	queries := dbsqlc.New(db)
	if queries == nil {
		return nil, fmt.Errorf("dbsqlc.New(): nil")
	}
	getKey := func(ctx context.Context, userID int64) (string, error) {
		key, err := queries.GetAPIKeysByUserID(ctx, userID)
		if err != nil {
			return "", fmt.Errorf("queries.GetUnassignedKey(ctx, userID): %w", err)
		}
		if len(key) == 0 {
			return "", fmt.Errorf("queries.GetAPIKeysByUserID(ctx, userID): no key found")
		}
		keyString := key[0].KeyString
		return keyString, nil
	}
	return getKey, nil
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
		magicLink := fmt.Sprintf("%s/auth/verify?onetime_token=%s", hostDomain, onetimeToken)
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

func NewSendInstruction(resendAPIKey string, emailDomain string, serviceDomain string) (SendInstructionFunc, error) {
	resendClient := resend.NewClient(resendAPIKey)
	if resendClient == nil {
		return nil, fmt.Errorf("resend.NewClient(): nil")
	}

	instructionTmpl, tmplErr := template.New("email").Parse(instructionHTML)
	if tmplErr != nil {
		return nil, fmt.Errorf("template.New('email').Parse(instructionHTML): nil")
	}

	sendInstruction := func(sendTo string, apiKey string) (string, error) {
		instructionData := InstructionData{
			APIKey:        apiKey,
			ServiceDomain: serviceDomain,
		}

		var instructionBodyBuffer bytes.Buffer
		if err := instructionTmpl.Execute(&instructionBodyBuffer, instructionData); err != nil {
			return "", fmt.Errorf("instructionTmpl.Execute: %w", err)
		}
		instructionBody := instructionBodyBuffer.String()

		params := &resend.SendEmailRequest{
			From:    fmt.Sprintf("API Keys <noreply@%s>", emailDomain),
			To:      []string{sendTo},
			Html:    instructionBody,
			Subject: "Your API Key",
		}

		sent, err := resendClient.Emails.Send(params)
		if err != nil {
			return "", fmt.Errorf("resendClient.Emails.Send: %w", err)
		}
		return sent.Id, nil
	}

	return sendInstruction, nil
}
