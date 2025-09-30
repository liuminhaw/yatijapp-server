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
