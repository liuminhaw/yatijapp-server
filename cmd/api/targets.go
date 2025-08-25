package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/liuminhaw/sessions-of-life/internal/data"
	"github.com/liuminhaw/sessions-of-life/internal/tokenizer"
	"github.com/liuminhaw/sessions-of-life/internal/validator"
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
		Title:       input.Title,
		Description: input.Description,
		Notes:       input.Notes,
		Status:      input.Status,
	}

	// Input validation
	v := validator.New()
	if data.ValidateTarget(v, &target); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	fts := data.GenTargetFTS(
		target.Title,
		target.Description,
		target.Notes,
		app.models.Targets.Jieba,
	)

	user := app.contextGetUser(r)
	err = app.models.Targets.Insert(&target, fts, user.UUID)
	if err != nil {
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

	target, err := app.models.Targets.Get(id)
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

	target, err := app.models.Targets.Get(id)
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
		target.Title = *input.Title
	}
	if input.Description != nil {
		target.Description = *input.Description
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
	if data.ValidateTarget(v, target); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.Targets.Update(target)
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
		app.notFoundResponse(w, r)
		return
	}

	err = app.models.Targets.Delete(id)
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

	input.Search = app.readString(qs, "search", "")
	input.Filters.Status = data.Status(app.readString(qs, "status", ""))
	input.Filters.Page = app.readInt(qs, "page", 1, v)
	input.Filters.PageSize = app.readInt(qs, "page_size", 20, v)
	input.Filters.Sort = app.readString(qs, "sort", "serial_id")

	input.Filters.SortSafelist = []string{
		"serial_id",
		"title",
		"created_at",
		"due_date",
		"updated_at",
		"-serial_id",
		"-title",
		"-created_at",
		"-due_date",
		"-updated_at",
	}
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
