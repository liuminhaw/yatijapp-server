package main

import (
	"net/http"

	"github.com/liuminhaw/yatijapp/internal/data"
	"github.com/liuminhaw/yatijapp/internal/tokenizer"
	"github.com/liuminhaw/yatijapp/internal/validator"
)

func (app *application) listRecordsHandler(w http.ResponseWriter, r *http.Request) {
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

	input.Filters.SortSafelist = []string{""}
	input.Filters.StatusSafelist = data.StatusFilterSafelist

	if data.ValidateFilters(v, input.Filters); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	t := tokenizer.New(input.Search, app.models.Records.Jieba)

	user := app.contextGetUser(r)
	records, metadata, err := app.models.Records.GetAll(*t, input.Filters, user.UUID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"records": records, "metadata": metadata}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}
