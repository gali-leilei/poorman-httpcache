// package main serves internal staff member
package main

import (
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

	// Parse template
	tmpl, err := template.New("form").Parse(formHTML)
	if err != nil {
		return fmt.Errorf("template.Parse: %w", err)
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
			tmpl.Execute(w, data)

		case "POST":
			// Handle form submission
			email := r.FormValue("email")
			if email == "" {
				data := FormData{Error: "Email is required"}
				tmpl.Execute(w, data)
				return
			}

			// Look up user by email
			user, err := queries.GetUserByEmail(ctx, email)
			if err != nil {
				logger.Error("Failed to get user by email", "email", email, "error", err)
				data := FormData{Email: email, Error: "User not found. Please contact support."}
				tmpl.Execute(w, data)
				return
			}

			// Get assigned API keys for the user
			apiKeys, err := queries.GetAssignedAPIKeysByUserID(ctx, user.ID)
			if err != nil {
				logger.Error("Failed to get API keys for user", "user_id", user.ID, "error", err)
				data := FormData{Email: email, Error: "Failed to retrieve API keys. Please contact support."}
				tmpl.Execute(w, data)
				return
			}

			if len(apiKeys) == 0 {
				data := FormData{Email: email, Error: "No API keys found for this user. Please contact support."}
				tmpl.Execute(w, data)
				return
			}

			// Use the first assigned API key
			apiKey := apiKeys[0].KeyString

			// Send email with API key using resend
			emailBody := fmt.Sprintf(`
				<h2>Your API Key</h2>
				<p>Hello,</p>
				<p>Your API key is: <strong>%s</strong></p>
				<p>Please keep this key secure and do not share it with others.</p>
				<p>Best regards,<br>The Team</p>
			`, apiKey)

			params := &resend.SendEmailRequest{
				From:    "API Keys <noreply@miromdind.online>",
				To:      []string{email},
				Html:    emailBody,
				Subject: "Your API Key",
			}

			sent, err := resendClient.Emails.Send(params)
			if err != nil {
				logger.Error("Failed to send email", "email", email, "error", err)
				data := FormData{Email: email, Error: "Failed to send email. Please try again later."}
				tmpl.Execute(w, data)
				return
			}

			logger.Info("API key email sent successfully", "email", email, "message_id", sent.Id)
			data := FormData{Success: "API key has been sent to your email address."}
			tmpl.Execute(w, data)
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
