// package main serves internal staff member
package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"httpcache/pkg"
	"httpcache/pkg/dbsqlc"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/resend/resend-go/v2"
)

// HTML template for the form
const formHTML = `
<!DOCTYPE html>
<html>
<head>
    <title>Request API Key</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 500px; margin: 50px auto; padding: 20px; }
        form { border: 1px solid #ccc; padding: 20px; border-radius: 5px; }
        input[type="email"] { width: 100%; padding: 8px; margin: 10px 0; border: 1px solid #ccc; border-radius: 3px; }
        input[type="submit"] { background-color: #4CAF50; color: white; padding: 10px 20px; border: none; border-radius: 3px; cursor: pointer; }
        input[type="submit"]:hover { background-color: #45a049; }
        .error { color: red; margin: 10px 0; }
        .success { color: green; margin: 10px 0; }
    </style>
</head>
<body>
    <h1>Request Your API Key</h1>
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
    {{if .Success}}<div class="success">{{.Success}}</div>{{end}}
    <form method="post">
        <label for="email">Email Address:</label>
        <input type="email" id="email" name="email" required value="{{.Email}}">
        <input type="submit" value="Send API Key">
    </form>
</body>
</html>
`

// HTML template for the email body
const emailHTML = `
<h2>Your API Key</h2>
<p>Hello,</p>
<p>Your API key is: <strong>{{.APIKey}}</strong></p>
<p>Please keep this key secure and do not share it with others.</p>

<h3>Bash Examples</h3>
<p>Use your API key to access our cached proxy services:</p>

<h4>Jina AI</h4>
<pre style="background-color: #f6f8fa; padding: 16px; border-radius: 6px; border: 1px solid #d1d9e0; overflow-x: auto; font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace; font-size: 85%;">
curl --location "https://cachev1.{{.EmailDomain}}/jina/https://www.example.com" \
  --header "Authorization: Bearer {{.APIKey}}"</pre>

<h4>Serper</h4>
<pre style="background-color: #f6f8fa; padding: 16px; border-radius: 6px; border: 1px solid #d1d9e0; overflow-x: auto; font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace; font-size: 85%;">
curl --location "https://cachev1.{{.EmailDomain}}/serper/search" \
  --header "X-API-KEY: {{.APIKey}}" \
  --header "Content-Type: application/json" \
  --data '{"q": "your search query"}'</pre>

<h3>Python Example:</h3>
<p>Here are Python examples for using the endpoints:</p>

<h4>Jina AI</h4>
<pre style="background-color: #f6f8fa; padding: 16px; border-radius: 6px; border: 1px solid #d1d9e0; overflow-x: auto; font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace; font-size: 85%;">
import requests

url = "https://cachev1.{{.EmailDomain}}/jina/https://www.example.com"
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

url = "https://cachev1.{{.EmailDomain}}/serper/search"
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
JINA_BASE_URL=https://cachev1.{{.EmailDomain}}/jina/
JINA_API_KEY={{.APIKey}}
SERPER_BASE_URL=https://cachev1.{{.EmailDomain}}/serper/
SERPER_API_KEY={{.APIKey}}

// in config.yaml file
env:
	JINA_BASE_URL: https://cachev1.{{.EmailDomain}}/jina/
	JINA_API_KEY: {{.APIKey}}
	SERPER_BASE_URL: https://cachev1.{{.EmailDomain}}/serper/
	SERPER_API_KEY: {{.APIKey}}
</pre>

<p>Best regards,<br>The Team</p>
`

func run(ctx context.Context, cfg pkg.Config, logger *slog.Logger) error {
	// Create a single HTTP server with path-based routing
	mux := chi.NewRouter()

	// Connect to database
	db, err := pgx.Connect(ctx, cfg.PostgresURL)
	if err != nil {
		return fmt.Errorf("pgx.Connect: %w", err)
	}
	defer db.Close(ctx)

	queries := dbsqlc.New(db)

	// Create resend client
	resendClient := resend.NewClient(cfg.ResendAPIKey)

	// Parse templates
	formTmpl, err := template.New("form").Parse(formHTML)
	if err != nil {
		return fmt.Errorf("form template.Parse: %w", err)
	}

	emailTmpl, err := template.New("email").Parse(emailHTML)
	if err != nil {
		return fmt.Errorf("email template.Parse: %w", err)
	}

	mux.HandleFunc("/request", func(w http.ResponseWriter, r *http.Request) {
		type FormData struct {
			Email   string
			Error   string
			Success string
		}

		switch r.Method {
		case "GET":
			// Display the form
			data := FormData{}
			err := formTmpl.Execute(w, data)
			if err != nil {
				logger.Error("Failed to execute form template", "error", err)
			}

		case "POST":
			// Handle form submission
			email := r.FormValue("email")
			if email == "" {
				data := FormData{Error: "Email is required"}
				err := formTmpl.Execute(w, data)
				if err != nil {
					logger.Error("Failed to execute form template", "error", err)
				}
				return
			}

			// Look up user by email
			// user, err := queries.GetUserByEmail(ctx, email)
			// if err != nil {
			// 	logger.Error("Failed to get user by email", "email", email, "error", err)
			// 	data := FormData{Email: email, Error: "User not found. Please contact support."}
			// 	tmpl.Execute(w, data)
			// 	return
			// }

			// // Get assigned API keys for the user
			// apiKeys, err := queries.GetAssignedAPIKeysByUserID(ctx, user.ID)
			// if err != nil {
			// 	logger.Error("Failed to get API keys for user", "user_id", user.ID, "error", err)
			// 	data := FormData{Email: email, Error: "Failed to retrieve API keys. Please contact support."}
			// 	tmpl.Execute(w, data)
			// 	return
			// }

			// if len(apiKeys) == 0 {
			// 	data := FormData{Email: email, Error: "No API keys found for this user. Please contact support."}
			// 	tmpl.Execute(w, data)
			// 	return
			// }

			// // Use the first assigned API key
			// apiKey := apiKeys[0].KeyString

			// for cachev1, use the internal key, but we still check for user email
			// Look up user by email
			_, err := queries.GetUserByEmail(ctx, email)
			if err != nil {
				logger.Error("Failed to get user by email", "email", email, "error", err)
				data := FormData{Email: email, Error: "User not found. Please contact support."}
				err = formTmpl.Execute(w, data)
				if err != nil {
					logger.Error("Failed to execute form template", "error", err)
				}
				return
			}
			apiKey := cfg.InternalKey

			// Generate email body using template
			type EmailData struct {
				APIKey      string
				EmailDomain string
			}

			emailData := EmailData{
				APIKey:      apiKey,
				EmailDomain: cfg.EmailDomain,
			}

			var emailBodyBuffer bytes.Buffer
			if err := emailTmpl.Execute(&emailBodyBuffer, emailData); err != nil {
				logger.Error("Failed to execute email template", "error", err)
				data := FormData{Email: email, Error: "Failed to generate email. Please try again later."}
				err = formTmpl.Execute(w, data)
				if err != nil {
					logger.Error("Failed to execute form template", "error", err)
				}
				return
			}
			emailBody := emailBodyBuffer.String()

			params := &resend.SendEmailRequest{
				From:    fmt.Sprintf("API Keys <noreply@%s>", cfg.EmailDomain),
				To:      []string{email},
				Html:    emailBody,
				Subject: "Your API Key",
			}

			sent, err := resendClient.Emails.Send(params)
			if err != nil {
				logger.Error("Failed to send email", "email", email, "error", err)
				data := FormData{Email: email, Error: "Failed to send email. Please try again later."}
				err = formTmpl.Execute(w, data)
				if err != nil {
					logger.Error("Failed to execute form template", "error", err)
				}
				return
			}

			logger.Info("API key email sent successfully", "email", email, "message_id", sent.Id)
			data := FormData{Success: "API key has been sent to your email address."}
			err = formTmpl.Execute(w, data)
			if err != nil {
				logger.Error("Failed to execute form template", "error", err)
			}
		}
	})

	// Single server listening on port 8080
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	// Start the single server
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed", "error", err)
			return
		}
	}()

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("Received shutdown signal, shutting down server...")

	// Create a context with a timeout for graceful shutdown
	shutdownCtx := context.Background()
	shutdownCtx, shutdownCancel := context.WithTimeout(shutdownCtx, 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error shutting down server", "error", err)
		return err
	}
	return nil
}

func main() {
	// parse with generics
	cfg, err := pkg.GetConfig()
	if err != nil {
		// Can't use logger here since it hasn't been created yet
		slog.Error("Failed to parse config", "error", err)
		os.Exit(1)
	}
	logger := pkg.GetLogger(cfg.LogLevel)
	logger.Info("Config", "cfg", cfg)
	ctx := context.Background()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("Failed to run", "error", err)
		os.Exit(1)
	}
}
