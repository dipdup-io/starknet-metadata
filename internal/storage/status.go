package storage

// Status -
type Status string

// defined statuses
const (
	StatusNew     Status = "new"
	StatusFilled  Status = "filled"
	StatusFailed  Status = "failed"
	StatusSuccess Status = "success"
)
