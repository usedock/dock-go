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

// Error codes the API returns for connectors and images. Compare with
// Error.Code.
const (
	// CodeConnectorNotConnected (HTTP 409): a requested connector is not
	// connected in the workspace.
	CodeConnectorNotConnected = "connector_not_connected"
	// CodeConnectorAdminOnly (HTTP 403): only organization admins can launch
	// or open docks that use connectors.
	CodeConnectorAdminOnly = "connector_admin_only"
	// CodeConnectorNotFound (HTTP 404): Disconnect of a connector that is not
	// connected.
	CodeConnectorNotFound = "connector_not_found"
	// CodeImageUnknown (HTTP 400): the image is not in the catalog.
	CodeImageUnknown = "image_unknown"
	// CodeImageUnavailable (HTTP 400): the image is known but no host serves it.
	CodeImageUnavailable = "image_unavailable"
)
