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

func (app *application) createActionHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TargetUUID  uuid.UUID      `json:"target_uuid"`
		DueDate     data.InputDate `json:"due_date"`
		Title       string         `json:"title"`
		Description string         `json:"description"`
		Notes       string         `json:"notes"`
		Status      data.Status    `json:"status"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	action := data.Action{
		TargetUUID:  input.TargetUUID,
		DueDate:     sql.NullTime(input.DueDate),
		Title:       input.Title,
		Description: input.Description,
		Notes:       input.Notes,
		Status:      input.Status,
	}

	v := validator.New()
	if data.ValidateAction(v, &action); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	user := app.contextGetUser(r)

	quota := data.DailyQuota{
		UsageDate: time.Now().UTC(),
		Resource:  "action",
		Limit:     app.config.user.dailyActionsCreationLimit,
	}

	err = app.models.CreateAction(&action, &quota, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		case errors.Is(err, data.ErrQuotaExceeded):
			msg := fmt.Sprintf(
				"action creation quota reached (%d per day, renew on midnight UTC)",
				quota.Limit,
			)
			app.quotaExceededResponse(w, r, msg)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/actions/%s", action.UUID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"action": action}, headers)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) showActionHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	user := app.contextGetUser(r)
	action, err := app.models.Actions.Get(id, user.UUID, "viewer")
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"action": action}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) updateActionHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	user := app.contextGetUser(r)
	action, err := app.models.Actions.Get(id, user.UUID, "editor")
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
		Title       *string         `json:"title"`
		Description *string         `json:"description"`
		Notes       *string         `json:"notes"`
		DueDate     *data.InputDate `json:"due_date"`
		Status      *data.Status    `json:"status"`
		TargetUUID  *uuid.UUID      `json:"target_uuid"`
	}
	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if input.Title != nil {
		action.Title = *input.Title
	}
	if input.Description != nil {
		action.Description = *input.Description
	}
	if input.Notes != nil {
		action.Notes = *input.Notes
	}
	if input.DueDate != nil {
		action.DueDate = sql.NullTime(*input.DueDate)
	}
	if input.Status != nil {
		action.Status = *input.Status
	}
	if input.TargetUUID != nil {
		action.TargetUUID = *input.TargetUUID
	}

	v := validator.New()
	if data.ValidateAction(v, action); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	fts := data.GenFTS(
		action.Title,
		action.Description,
		action.Notes,
		app.models.Actions.Jieba,
	)

	err = app.models.Actions.Update(action, fts, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrEditConflict):
			app.editConflictResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"action": action}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) deleteActionHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := app.contextGetUser(r)
	err = app.models.Actions.Delete(id, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "action successfully deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) listActionsHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		search string
		data.Filters
	}

	v := validator.New()

	qs := r.URL.Query()
	input.search = app.readString(qs, "search", "")
	input.Filters.Status = data.Status(app.readString(qs, "status", ""))
	input.Filters.Page = app.readInt(qs, "page", 1, v)
	input.Filters.PageSize = app.readInt(qs, "page_size", 20, v)
	input.Filters.Sort = app.readString(qs, "sort", "-last_active")
	input.Filters.SortSafelist = data.SortSafelist
	input.Filters.StatusSafelist = data.StatusFilterSafelist

	if data.ValidateFilters(v, input.Filters); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	t := tokenizer.New(input.search, app.models.Actions.Jieba)

	user := app.contextGetUser(r)
	actions, metadata, err := app.models.Actions.GetAll(
		*t,
		input.Filters,
		uuid.NullUUID{Valid: false},
		user.UUID,
	)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(
		w,
		http.StatusOK,
		envelope{"actions": actions, "metadata": metadata},
		nil,
	)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func (app *application) listActionSessionsHandler(w http.ResponseWriter, r *http.Request) {
	actionUUID, err := app.readUUIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	var input struct {
		search  string
		Filters data.Filters
	}

	v := validator.New()

	qs := r.URL.Query()
	input.search = app.readString(qs, "search", "")
	input.Filters.Status = data.Status(app.readString(qs, "status", ""))
	input.Filters.Page = app.readInt(qs, "page", 1, v)
	input.Filters.PageSize = app.readInt(qs, "page_size", 20, v)
	input.Filters.Sort = app.readString(qs, "sort", "-updated_at")
	input.Filters.SortSafelist = data.SortSafelist
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
		uuid.NullUUID{Valid: true, UUID: actionUUID},
		user.UUID,
	)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(
		w,
		http.StatusOK,
		envelope{"sessions": sessions, "metadata": metadata},
		nil,
	)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}
