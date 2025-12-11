package data

type Status string

const (
	StatusQueued     Status = "queued"
	StatusInProgress Status = "in progress"
	StatusComplete   Status = "completed"
	StatusCanceled   Status = "canceled"
	StatusArchived   Status = "archived"
	StatusAny        Status = "" // Used for filtering only (not valid for target status)
)

var StatusSafelist = []Status{
	StatusQueued,
	StatusInProgress,
	StatusComplete,
	StatusCanceled,
	StatusArchived,
}

var SessionStatusSafelist = []Status{
	StatusInProgress,
	StatusComplete,
}

var StatusFilterSafelist = []Status{
	StatusQueued,
	StatusInProgress,
	StatusComplete,
	StatusCanceled,
	StatusArchived,
	StatusAny, // Allow 'any' for filtering purposes
}

var SortSafelist = []string{
	"serial_id",
	"title",
	"created_at",
	"due_date",
	"last_active",
	"updated_at",
	"-serial_id",
	"-title",
	"-created_at",
	"-due_date",
	"-last_active",
	"-updated_at",
}

var SessionSortSafelist = []string{
	"starts_at",
	"ends_at",
	"created_at",
	"updated_at",
	"-starts_at",
	"-ends_at",
	"-created_at",
	"-updated_at",
}

var SortOrderSafelist = []string{
	"ascending",
	"descending",
}

func StringSliceToStatusSlice(input []string) []Status {
	statuses := make([]Status, len(input))
	for i, s := range input {
		statuses[i] = Status(s)
	}
	return statuses
}
