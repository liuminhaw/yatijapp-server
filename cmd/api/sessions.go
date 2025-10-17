package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/liuminhaw/yatijapp/internal/data"
	"github.com/liuminhaw/yatijapp/internal/tokenizer"
	"github.com/liuminhaw/yatijapp/internal/validator"
)

func (app *application) createSessionHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		StartsAt   time.Time    `json:"starts_at"`
		EndsAt     sql.NullTime `json:"ends_at"`
		Notes      string       `json:"notes"`
		ActionUUID uuid.UUID    `json:"action_uuid"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	session := data.Session{
		StartsAt:   input.StartsAt,
		EndsAt:     input.EndsAt,
		Notes:      input.Notes,
		ActionUUID: input.ActionUUID,
	}

	v := validator.New()
	if data.ValidateSession(v, &session); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	user := app.contextGetUser(r)

	quota := data.DailyQuota{
		UsageDate: time.Now().UTC(),
		Resource:  "session",
		Limit:     app.config.user.dailySessionsCreationLimit,
	}

	err = app.models.CreateSession(&session, &quota, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		case errors.Is(err, data.ErrQuotaExceeded):
			msg := fmt.Sprintf(
				"session creation quota reached (%d per day, renew on midnight UTC)",
				quota.Limit,
			)
			app.quotaExceededResponse(w, r, msg)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/sessions/%s", session.UUID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"session": session}, headers)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) showSessionHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	user := app.contextGetUser(r)
	session, err := app.models.Sessions.Get(id, user.UUID, "viewer")
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"session": session}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) updateSessionHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	user := app.contextGetUser(r)
	session, err := app.models.Sessions.Get(id, user.UUID, "editor")
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	var input struct {
		StartsAt   *time.Time    `json:"starts_at"`
		EndsAt     *sql.NullTime `json:"ends_at"`
		Notes      *string       `json:"notes"`
		ActionUUID *uuid.UUID    `json:"action_uuid,omitzero"`
	}
	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if input.StartsAt != nil {
		session.StartsAt = *input.StartsAt
	}
	if input.EndsAt != nil {
		session.EndsAt = *input.EndsAt
	}
	if input.Notes != nil {
		session.Notes = *input.Notes
	}
	if input.ActionUUID != nil {
		session.ActionUUID = *input.ActionUUID
	}

	v := validator.New()
	if data.ValidateSession(v, session); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	fts := data.GenFTS("", "", session.Notes, app.models.Sessions.Jieba)

	err = app.models.Sessions.Update(session, fts, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrEditConflict):
			app.editConflictResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"session": session}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) deleteSessionHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := app.contextGetUser(r)
	err = app.models.Sessions.Delete(id, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "session successfully deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) listSessionsHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		search string
		data.Filters
	}

	v := validator.New()

	qs := r.URL.Query()
	input.search = app.readString(qs, "search", "")
	input.Filters.Page = app.readInt(qs, "page", 1, v)
	input.Filters.PageSize = app.readInt(qs, "page_size", 20, v)
	input.Filters.Sort = app.readString(qs, "sort", "-starts_at")
	// input.Filters.Status = data.StatusAny

	input.Filters.SortSafelist = []string{
		"starts_at",
		"ends_at",
		"created_at",
		"last_active",
		"-starts_at",
		"-ends_at",
		"-created_at",
		"-last_active",
	}
	input.Filters.StatusSafelist = data.StatusFilterSafelist

	if data.ValidateFilters(v, input.Filters); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	t := tokenizer.New(input.search, app.models.Sessions.Jieba)

	user := app.contextGetUser(r)
	sessions, metadata, err := app.models.Sessions.GetAll(
		*t,
		input.Filters,
		uuid.NullUUID{Valid: false},
		user.UUID,
	)
	if err != nil {
		app.logger.Info("Query error")
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(
		w, http.StatusOK, envelope{"sessions": sessions, "metadata": metadata}, nil,
	)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}
