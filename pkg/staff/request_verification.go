package staff

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"
)

// HTML template for the form
//
//nolint:lll
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

type FormData struct {
	Email   string
	Error   string
	Success string
}

type Form struct {
	template *template.Template
}

func NewForm() (*Form, error) {
	formTmpl, err := template.New("form").Parse(formHTML)
	if err != nil {
		return nil, fmt.Errorf("template.New('form').Parse(formHTML): %w", err)
	}
	return &Form{
		template: formTmpl,
	}, nil
}

func (f *Form) Success(w http.ResponseWriter, email string, success string) error {
	data := FormData{Email: email, Success: success}
	err := f.template.Execute(w, data)
	if err != nil {
		return fmt.Errorf("template.Execute: %w", err)
	}
	return nil
}

func (f *Form) Error(w http.ResponseWriter, email string, error string) error {
	data := FormData{Email: email, Error: error}
	err := f.template.Execute(w, data)
	if err != nil {
		return fmt.Errorf("template.Execute: %w", err)
	}
	return nil
}

func ShowVerificationFormHandler(form *Form, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := form.Success(w, "", "")
		if err != nil {
			logger.Error("Failed to execute form template", "error", err)
			return
		}
	}
}

func RequestVerificationHandler(sm *scs.SessionManager, form *Form, allowList AllowlistFunc, sendMail SendMailFunc, nextPath string, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// check if user is already logged in
		isAuthorized := sm.GetString(r.Context(), "authorized")
		if isAuthorized == "true" {
			http.Redirect(w, r, nextPath, http.StatusSeeOther)
			return
		}

		// get email from form submission
		email := r.FormValue("email")
		if email == "" {
			err := form.Error(w, email, "Email is required")
			if err != nil {
				logger.Error("Failed to execute form template", "error", err)
			}
			return
		}

		//Look up user by email
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

		// Generate magic link and store it
		randomToken, err := generateToken()
		if err != nil {
			logger.Error("Failed to generate one-time token", "error", err)
			err = form.Error(w, email, "Failed to generate one-time token. Please try again later.")
			if err != nil {
				logger.Error("Failed to execute form template", "error", err)
			}
			return
		}
		sm.Put(r.Context(), "onetime_token", randomToken)
		sm.Put(r.Context(), "user_id", strconv.FormatInt(userID, 10))
		sm.Put(r.Context(), "email", email)

		sentID, err := sendMail(email, randomToken)
		if err != nil {
			logger.Error("Failed to send email", "error", err)
			err = form.Error(w, email, "Failed to send email. Please try again later.")
			if err != nil {
				logger.Error("Failed to execute form template", "error", err)
			}
			return
		}

		logger.Info("Secure login link sent successfully", "email", email, "message_id", sentID)
		err = form.Success(w, email, "Secure login link has been sent to your email address.")
		if err != nil {
			logger.Error("Failed to execute form template", "error", err)
			return
		}
	}
}
