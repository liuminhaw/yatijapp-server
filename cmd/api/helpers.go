package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/julienschmidt/httprouter"
	"github.com/liuminhaw/sessions-of-life/internal/validator"
)

func (app *application) readUUIDParam(r *http.Request) (uuid.UUID, error) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := uuid.FromString(params.ByName("uuid"))
	if err != nil {
		return uuid.Nil, errors.New("invalid UUID parameter")
	}

	return id, nil
}

type envelope map[string]any

func (app *application) writeJSON(
	w http.ResponseWriter,
	status int,
	data envelope,
	headers http.Header,
) error {
	js, err := json.Marshal(data)
	if err != nil {
		return err
	}
	js = append(js, '\n')

	maps.Copy(w.Header(), headers)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)

	return nil
}

func (app *application) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	// Limit the size of the request body to 1 MB
	r.Body = http.MaxBytesReader(w, r.Body, 1_048_576) // 1 MB limit

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(dst)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError
		var maxBytesError *http.MaxBytesError

		switch {
		case errors.As(err, &syntaxError):
			return fmt.Errorf(
				"body contains badly-formed JSON (at position %d)",
				syntaxError.Offset,
			)
		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf(
					"body contains incorrect JSON type for field %q",
					unmarshalTypeError.Field,
				)
			}
			return fmt.Errorf(
				"body contains incorrect JSON type (at character %d)",
				unmarshalTypeError.Offset,
			)
		case strings.Contains(err.Error(), "parsing time"):
			return errors.New(
				"body contains incorrectly formatted time value, expected RFC3339 format",
			)
		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("body contains unknown key %s", fieldName)
		case errors.As(err, &maxBytesError):
			return fmt.Errorf("body must not be larger than %d bytes", maxBytesError.Limit)
		// A json.InvalidUnmarshalError error will be returned if we pass something
		// that is not a non-nil pointer as the target destination to Decode().
		case errors.As(err, &invalidUnmarshalError):
			panic(err)
		default:
			return err
		}
	}

	// Check if there are any additional content after json body.
	err = dec.Decode(&struct{}{})
	if !errors.Is(err, io.EOF) {
		return errors.New("body must only contain a single JSON value")
	}

	return nil
}

// readString() returns a string value from the query string, or the provided
// default value if no matching key is found.
func (app *application) readString(qs url.Values, key string, defaultValue string) string {
	s := qs.Get(key)
	if s == "" {
		return defaultValue
	}

	return s
}

// readInt() helper reads a string value from the query string and converts it to 
// an integer before returning. Returns the provided default value if no matching 
// key is found. If the conversion fails, we record an error message to the provided
// validator.Validator instance.
func (app *application) readInt(
	qs url.Values,
	key string,
	defaultValue int,
	v *validator.Validator,
) int {
	s := qs.Get(key)
	if s == "" {
		return defaultValue
	}

	i, err := strconv.Atoi(s)
	if err != nil {
		v.AddError(key, "must be an integer")
		return defaultValue
	}

	return i
}

// readCSV() reads a string value from the query string, splits if into a slice using
// comma character. If no matching key is found, it returns the provided default value.
// func (app *application) readCSV(qs url.Values, key string, defaultValue []string) []string {
// 	csv := qs.Get(key)
// 	if csv == "" {
// 		return defaultValue
// 	}
//
// 	return strings.Split(csv, ",")
// }
