// Package dock is the Go client for the Dock control plane API (/v1).
//
// Create a client with [NewClient]; it reads DOCK_API_KEY and DOCK_API_URL
// unless options override them. Every call takes a [context.Context].
// API failures are returned as *[Error], which carries the HTTP status and the
// stable machine-readable code:
//
//	var apiErr *dock.Error
//	if errors.As(err, &apiErr) && apiErr.Code == "dock_not_found" {
//		// ...
//	}
package dock
