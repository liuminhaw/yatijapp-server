package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/liuminhaw/yatijapp/internal/data"
	"github.com/liuminhaw/yatijapp/internal/tokenizer"
	"github.com/liuminhaw/yatijapp/internal/validator"
)

func (app *application) createTargetHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
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

	target := data.Target{
		DueDate:     sql.NullTime(input.DueDate),
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		Notes:       input.Notes,
		Status:      input.Status,
	}

	// Input validation
	v := validator.New()
	if data.ValidateTarget(v, &target, "create"); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	user := app.contextGetUser(r)

	quota := data.DailyQuota{
		UsageDate: time.Now().UTC(),
		Resource:  "target",
		Limit:     app.config.user.dailyTargetsCreationLimit,
	}

	err = app.models.CreateTarget(&target, &quota, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrQuotaExceeded):
			msg := fmt.Sprintf(
				"target creation quota reached (%d per day, renew on midnight UTC)",
				quota.Limit,
			)
			app.quotaExceededResponse(w, r, msg)
		}
		app.serverErrorResponse(w, r, err)
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/targets/%s", target.UUID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"target": target}, headers)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) showTargetHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	user := app.contextGetUser(r)
	target, err := app.models.Targets.Get(id, user.UUID, "viewer")
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"target": target}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) updateTargetHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	user := app.contextGetUser(r)
	target, err := app.models.Targets.Get(id, user.UUID, "editor")
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
		target.Title = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		target.Description = strings.TrimSpace(*input.Description)
	}
	if input.Notes != nil {
		target.Notes = *input.Notes
	}
	if input.DueDate != nil {
		target.DueDate = sql.NullTime(*input.DueDate)
	}
	if input.Status != nil {
		target.Status = *input.Status
	}

	v := validator.New()
	if data.ValidateTarget(v, target, "update"); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	fts := data.GenFTS(
		target.Title,
		target.Description,
		target.Notes,
		app.models.Targets.Jieba,
	)

	err = app.models.Targets.Update(target, fts, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrEditConflict):
			app.editConflictResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"target": target}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) deleteTargetHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readUUIDParam(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := app.contextGetUser(r)
	err = app.models.Targets.Delete(id, user.UUID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "target successfully deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) listTargetsHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Search string
		data.Filters
	}

	v := validator.New()

	qs := r.URL.Query()
	statuses := app.readCSV(qs, "status", []string{})

	input.Search = app.readString(qs, "search", "")
	input.Filters.Status = data.StringSliceToStatusSlice(statuses)
	input.Filters.Page = app.readInt(qs, "page", 1, v)
	input.Filters.PageSize = app.readInt(qs, "page_size", 20, v)
	input.Filters.Sort = app.readString(qs, "sort", "-last_active")

	input.Filters.SortSafelist = data.SortSafelist
	input.Filters.StatusSafelist = data.StatusFilterSafelist

	if data.ValidateFilters(v, input.Filters); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	t := tokenizer.New(input.Search, app.models.Targets.Jieba)

	user := app.contextGetUser(r)
	targets, metadata, err := app.models.Targets.GetAllForUser(*t, input.Filters, user.UUID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"targets": targets, "metadata": metadata}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func (app *application) listTargetActionsHandler(w http.ResponseWriter, r *http.Request) {
	targetUUID, err := app.readUUIDParam(r)
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
	statuses := app.readCSV(qs, "status", []string{})

	input.search = app.readString(r.URL.Query(), "search", "")
	input.Filters.Status = data.StringSliceToStatusSlice(statuses)
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
		uuid.NullUUID{Valid: true, UUID: targetUUID},
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
