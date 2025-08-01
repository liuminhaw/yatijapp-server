package data

type Status string

const (
	StatusQueued     Status = "queued"
	StatusInProgress Status = "in progress"
	StatusComplete   Status = "completed"
	StatusCanceled   Status = "canceled"
	StatusAny        Status = "" // Used for filtering only (not valid for target status)
)

var StatusSafelist = []Status{
	StatusQueued,
	StatusInProgress,
	StatusComplete,
	StatusCanceled,
}

var StatusFilterSafelist = []Status{
	StatusQueued,
	StatusInProgress,
	StatusComplete,
	StatusCanceled,
	StatusAny, // Allow 'any' for filtering purposes
}

// const (
// 	_ Status = iota // Skip zero value
// 	StatusQueued
// 	StatusInProgress
// 	StatusComplete
// 	StatusCanceled
// )

// func (s Status) MarshalJSON() ([]byte, error) {
// 	var jsonValue string
// 	switch s {
// 	case StatusQueued:
// 		jsonValue = "queued"
// 	case StatusInProgress:
// 		jsonValue = "in progress"
// 	case StatusComplete:
// 		jsonValue = "complete"
// 	case StatusCanceled:
// 		jsonValue = "canceled"
// 	default:
// 		return nil, errors.New("invalid status value")
// 	}
//
// 	quotedJSONValue := strconv.Quote(jsonValue)
// 	return []byte(quotedJSONValue), nil
// }
