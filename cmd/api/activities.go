package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/gofrs/uuid/v5"
	"github.com/liuminhaw/sessions-of-life/internal/data"
	"github.com/liuminhaw/sessions-of-life/internal/tokenizer"
	"github.com/liuminhaw/sessions-of-life/internal/validator"
)

func (app *application) createActivityHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TargetUUID  uuid.UUID      `json:"target_id"`
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

	activity := data.Activity{
		TargetUUID:  input.TargetUUID,
		DueDate:     sql.NullTime(input.DueDate),
		Title:       input.Title,
		Description: input.Description,
		Notes:       input.Notes,
		Status:      input.Status,
	}

	v := validator.New()
	if data.ValidateActivity(v, &activity); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	fts := data.GenFTS(
		activity.Title,
		activity.Description,
		activity.Notes,
		app.models.Activities.Jieba,
	)

	user := app.contextGetUser(r)
	err = app.models.Activities.Insert(&activity, fts, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/activities/%s", activity.UUID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"activity": activity}, headers)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) showActivityHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	app.logger.Info("showActivityHandler id", "id", id)
	user := app.contextGetUser(r)
	activity, err := app.models.Activities.Get(id, user.UUID, "viewer")
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"activity": activity}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) updateActivityHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	user := app.contextGetUser(r)
	activity, err := app.models.Activities.Get(id, user.UUID, "editor")
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
	}
	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if input.Title != nil {
		activity.Title = *input.Title
	}
	if input.Description != nil {
		activity.Description = *input.Description
	}
	if input.Notes != nil {
		activity.Notes = *input.Notes
	}
	if input.DueDate != nil {
		activity.DueDate = sql.NullTime(*input.DueDate)
	}
	if input.Status != nil {
		activity.Status = *input.Status
	}

	v := validator.New()
	if data.ValidateActivity(v, activity); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.Activities.Update(activity, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrEditConflict):
			app.editConflictResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"activity": activity}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) deleteActivityHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := app.contextGetUser(r)
	err = app.models.Activities.Delete(id, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "activity successfully deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) listActivitiesHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Search string
		data.Filters
	}

	v := validator.New()

	qs := r.URL.Query()
	input.Search = app.readString(qs, "search", "")
	input.Filters.Status = data.Status(app.readString(qs, "status", ""))
	input.Filters.Page = app.readInt(qs, "page", 1, v)
	input.Filters.PageSize = app.readInt(qs, "page_size", 20, v)
	input.Filters.Sort = app.readString(qs, "sort", "last_active")

	input.Filters.SortSafelist = []string{
		"serial_id",
		"title",
		"created_at",
		"due_date",
		"last_active",
		"-serial_id",
		"-title",
		"-created_at",
		"-due_date",
		"-last_active",
	}
	input.Filters.StatusSafelist = data.StatusFilterSafelist

	if data.ValidateFilters(v, input.Filters); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	t := tokenizer.New(input.Search, app.models.Activities.Jieba)

	user := app.contextGetUser(r)
	activities, metadata, err := app.models.Activities.GetAll(*t, input.Filters, user.UUID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(
		w,
		http.StatusOK,
		envelope{"activities": activities, "metadata": metadata},
		nil,
	)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}
