package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/liuminhaw/sessions-of-life/internal/data"
	"github.com/liuminhaw/sessions-of-life/internal/validator"
)

type AuthenticationToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	SessionUUID  uuid.UUID `json:"session_id"`
}

func (app *application) generateAuthenticationToken(
	userUUID, sessionUUID uuid.UUID,
	accessTokenTTL, refreshTokenTTL time.Duration,
) (AuthenticationToken, error) {
	accessToken, err := app.models.Tokens.New(
		userUUID,
		sessionUUID,
		accessTokenTTL,
		data.ScopeAuthentication,
	)
	if err != nil {
		return AuthenticationToken{}, err
	}
	refreshToken, err := app.models.Tokens.New(
		userUUID,
		sessionUUID,
		refreshTokenTTL,
		data.ScopeRefresh,
	)
	if err != nil {
		return AuthenticationToken{}, err
	}

	return AuthenticationToken{
		AccessToken:  accessToken.Plaintext,
		RefreshToken: refreshToken.Plaintext,
		SessionUUID:  sessionUUID,
	}, nil
}

func (app *application) renewAuthenticationToken(
	token string,
	accessTokenTTL, refreshTokenTTL time.Duration,
) (AuthenticationToken, error) {
	currToken, err := app.models.Tokens.Get(token, data.ScopeRefresh)
	if err != nil {
		return AuthenticationToken{}, err
	}

	sessionUUID := currToken.SessionUUID
	return app.generateAuthenticationToken(
		currToken.UserUUID,
		sessionUUID,
		accessTokenTTL,
		refreshTokenTTL,
	)
}

// createActivationTokenHandler generates a new activation token for a user and
// sends it via email to the user.
func (app *application) createActivationTokenHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	v := validator.New()
	if data.ValidateEmail(v, input.Email); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	user, err := app.models.Users.GetByEmail(input.Email)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			v.AddError("email", "no matching email address found")
			app.failedValidationResponse(w, r, v.Errors)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if user.Activated {
		v.AddError("email", "user is already activated")
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	token, err := app.models.Tokens.New(user.UUID, uuid.Nil, 3*24*time.Hour, data.ScopeActivation)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.background(func() {
		data := map[string]any{
			"activationToken": token.Plaintext,
			"username":        user.Name,
		}

		err := app.mailer.Send(user.Email, "token_activation.tmpl", data)
		if err != nil {
			app.logger.Error(err.Error())
		}
	})

	env := envelope{"message": "an email will be sent to you containing activation instructions"}
	err = app.writeJSON(w, http.StatusAccepted, env, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func (app *application) createAuthenticationTokenHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	v := validator.New()
	data.ValidateEmail(v, input.Email)
	data.ValidatePasswordPlaintext(v, input.Password)
	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	user, err := app.models.Users.GetByEmail(input.Email)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.invalidCredentialsResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	match, err := user.Password.Matches(input.Password, app.config.pepper)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	if !match {
		app.invalidCredentialsResponse(w, r)
		return
	}

	// TODO: authentication token expiration time configuration
	sessionUUID, err := uuid.NewV7()
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	token, err := app.generateAuthenticationToken(
		user.UUID,
		sessionUUID,
		15*time.Minute,
		24*time.Hour,
	)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusCreated, envelope{
		"authentication_token": token,
	}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) refreshAuthenticationTokenHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refresh_token"`
	}
	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	v := validator.New()
	if data.ValidateTokenPlaintext(v, input.RefreshToken); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	token, err := app.renewAuthenticationToken(input.RefreshToken, 15*time.Minute, 24*time.Hour)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	if err := app.models.Tokens.Delete(input.RefreshToken, data.ScopeRefresh); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusCreated, envelope{
		"authentication_token": token,
	}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// createPasswordResetTokenHandler generates a new password reset token for a user and
// sends it via email to the user.
func (app *application) createPasswordResetTokenHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	v := validator.New()
	if data.ValidateEmail(v, input.Email); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	user, err := app.models.Users.GetByEmail(input.Email)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			v.AddError("email", "no matching email address found")
			app.failedValidationResponse(w, r, v.Errors)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if !user.Activated {
		v.AddError("email", "user account must be activated")
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	token, err := app.models.Tokens.New(
		user.UUID,
		uuid.Nil,
		30*time.Minute,
		data.ScopePasswordReset,
	)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.background(func() {
		data := map[string]any{
			"username":   user.Name,
			"resetToken": token.Plaintext,
		}

		err := app.mailer.Send(user.Email, "token_password_reset.tmpl", data)
		if err != nil {
			app.logger.Error(err.Error())
		}
	})

	env := envelope{
		"message": "an email will be sent to you containing password reset instructions",
	}
	err = app.writeJSON(w, http.StatusAccepted, env, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
