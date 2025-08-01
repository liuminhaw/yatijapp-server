package data

import (
	"errors"
	"strconv"
	"time"
)

var ErrInvalidTimeFormat = errors.New("invalid time format")

var acceptedFormats = []string{
	time.RFC3339,
	time.RFC822Z,
	"02 Jan 2006 15:04 -0700",
	"2006-01-02 15:04:05 -0700",
	"2006/01/02 15:04:05 -0700",
}

type InputTime time.Time

// Implement a UnmarshalJSON method on the InputTime type so that it satisfies
// the json.Unmarshaler interface.
func (it *InputTime) UnmarshalJSON(jsonValue []byte) error {
	unquotedJSONValue, err := strconv.Unquote(string(jsonValue))
	if err != nil {
		return ErrInvalidTimeFormat
	}

	for _, format := range acceptedFormats {
		parsedTime, err := time.Parse(format, unquotedJSONValue)
		if err == nil {
			*it = (InputTime)(parsedTime)
			return nil
		}
	}

	return ErrInvalidTimeFormat
}

func (it InputTime) MarshalJSON() ([]byte, error) {
	// Format the time as RFC3339
	formattedTime := time.Time(it).Format(time.RFC3339)
	return []byte(strconv.Quote(formattedTime)), nil
}

func (it InputTime) Time() time.Time {
	return time.Time(it)
}

func (it InputTime) String() string {
	return it.Time().String()
}
