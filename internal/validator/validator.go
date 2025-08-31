package validator

import (
	"regexp"
	"slices"
	"unicode"
)

// regular expression for sanity checking the format of an email address
var EmailRX = regexp.MustCompile(
	"^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$",
)

// Validator contains a map of validation errors.
type Validator struct {
	Errors map[string]string
}

// New creates a Validator instance with an empty error map.
func New() *Validator {
	return &Validator{Errors: make(map[string]string)}
}

// Valid returns true if the errors map has no entries.
func (v *Validator) Valid() bool {
	return len(v.Errors) == 0
}

// AddError adds a validation error to the errors map if no entry for the key already exists.
func (v *Validator) AddError(key, message string) {
	if _, exists := v.Errors[key]; !exists {
		v.Errors[key] = message
	}
}

// Check add an error message to the map if validation fails.
func (v *Validator) Check(ok bool, key, message string) {
	if !ok {
		v.AddError(key, message)
	}
}

// PermittedValue checks if a value is in the list of permitted values.
func PermittedValue[T comparable](value T, permittedValues ...T) bool {
	return slices.Contains(permittedValues, value)
}

// Matches returns true if the value matches the regular expression.
func Matches(value string, rx *regexp.Regexp) bool {
	return rx.MatchString(value)
}

// Unique checks if all values in the slice are unique.
func Unique[T comparable](values []T) bool {
	uniqueValues := make(map[T]struct{})

	for _, value := range values {
		uniqueValues[value] = struct{}{}
	}

	return len(uniqueValues) == len(values)
}

// ValidUnicodeChars check if there are Control(Cc), Format(Cf), Bidi controls,
// and zero-width cahracters in the string.
// The input string should be normalized before calling this function.
func ValidUnicodeChars(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}

	return true
}
