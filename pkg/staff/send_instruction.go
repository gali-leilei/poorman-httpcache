package staff

import (
	"log/slog"
	"net/http"
)

func SendInstructionHandler(form *Form, allowList AllowlistFunc, idToKey IDToKeyFunc, sendInstruction SendInstructionFunc, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type FormData struct {
			Email   string
			Error   string
			Success string
		}

		switch r.Method {
		case "GET":
			// Display the form
			data := FormData{}
			err := form.template.Execute(w, data)
			if err != nil {
				logger.Error("Failed to execute form template", "error", err)
			}

		case "POST":
			// Handle form submission
			email := r.FormValue("email")
			if email == "" {
				err := form.Error(w, "", "Email is required")
				if err != nil {
					logger.Error("Failed to execute form template", "error", err)
				}
				return
			}

			// Look up user by email
			userID, err := allowList(r.Context(), email)

			if err != nil {
				// data := Form{Email: email, Error: "User not allowed to request secure login link. Please contact support."}
				// err = formTmpl.Execute(w, data)
				err = form.Error(w, email, "User not allowed to request secure login link. Please contact support.")
				if err != nil {
					logger.Error("Failed to execute form template", "error", err)
				}
				return
			}

			apiKey, err := idToKey(r.Context(), userID)
			if err != nil {
				logger.Error("Failed to get API key for user", "user_id", userID, "error", err)
				err = form.Error(w, email, "Failed to get API key for user. Please contact support.")
				if err != nil {
					logger.Error("Failed to execute form template", "error", err)
				}
				return
			}

			sentID, err := sendInstruction(email, apiKey)
			if err != nil {
				logger.Error("Failed to send email", "email", email, "error", err)
				err = form.Error(w, email, "Failed to send email. Please try again later.")
				if err != nil {
					logger.Error("Failed to execute form template", "error", err)
				}
				return
			}

			logger.Info("API key email sent successfully", "email", email, "message_id", sentID)
			err = form.Success(w, email, "API key has been sent to your email address.")
			if err != nil {
				logger.Error("Failed to execute form template", "error", err)
			}
		}
	}
}
