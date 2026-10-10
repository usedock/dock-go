package dock

import "fmt"

// Error is an API failure: a non-2xx response, or a successful response the
// SDK cannot use. Use errors.As to read it.
type Error struct {
	// Code is the stable machine-readable code, for example "dock_not_found".
	// When the response has no Dock error envelope it is "http_<status>".
	Code string
	// Status is the HTTP status of the response.
	Status int
	// Message is the human-readable message. Do not match on it.
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("dock: %s (HTTP %d)", e.Code, e.Status)
	}
	return fmt.Sprintf("dock: %s (HTTP %d): %s", e.Code, e.Status, e.Message)
}
