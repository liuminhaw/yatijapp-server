package data

import (
	"database/sql"
	"errors"
	"strconv"
	"time"
)

var ErrInvalidTimeFormat = errors.New("invalid date format, expected YYYY-mm-dd")

var acceptedFormats = []string{
	"2006-01-02",
}

// type InputDate time.Time
type InputDate sql.NullTime

// Implement a UnmarshalJSON method on the InputTime type so that it satisfies
// the json.Unmarshaler interface.
func (it *InputDate) UnmarshalJSON(jsonValue []byte) error {
	if string(jsonValue) == "null" {
		*it = InputDate(sql.NullTime{Valid: false})
		return nil
	}

	unquotedJSONValue, err := strconv.Unquote(string(jsonValue))
	if err != nil {
		return ErrInvalidTimeFormat
	}

	if unquotedJSONValue == "" {
		*it = InputDate(sql.NullTime{Valid: false})
		return nil
	}

	for _, format := range acceptedFormats {
		parsedTime, err := time.Parse(format, unquotedJSONValue)
		if err == nil {
			*it = InputDate(sql.NullTime{
				Time:  parsedTime,
				Valid: true,
			})
			return nil
		}
	}

	return ErrInvalidTimeFormat
}

func (it InputDate) MarshalJSON() ([]byte, error) {
	nullTime := sql.NullTime(it)
	if !nullTime.Valid {
		return []byte(""), nil
	}

	formattedTime := nullTime.Time.Format("2006-01-02")
	return []byte(strconv.Quote(formattedTime)), nil
}

func (it InputDate) GetTime() time.Time {
	return sql.NullTime(it).Time
}

func (it InputDate) String() string {
	nullTime := sql.NullTime(it)
	if !nullTime.Valid {
		return ""
	}

	return nullTime.Time.Format("2006-01-02")
}
