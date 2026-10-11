package dock

import (
	"context"
	"net/url"
	"strconv"
)

// Port visibility values for ExposePortParams.Visibility and Port.Visibility.
const (
	// VisibilityPrivate needs a dock-bound API key or an organization
	// member's sign-in to open the URL. It is the default for a new port.
	VisibilityPrivate = "private"
	// VisibilityPublic lets anyone with the URL open it.
	VisibilityPublic = "public"
)

// Port is an exposed dock port.
type Port struct {
	Port int `json:"port"`
	// Visibility is VisibilityPublic or VisibilityPrivate. Ports exposed
	// before visibility existed read as public.
	Visibility string `json:"visibility"`
	// URL serves the port; nil while hosted ports are off on the deployment.
	URL       *string `json:"url"`
	CreatedAt string  `json:"createdAt,omitempty"`
}

// ExposePortParams are the optional fields of PortsService.Expose.
type ExposePortParams struct {
	// Visibility is VisibilityPublic or VisibilityPrivate. Empty keeps the
	// current visibility of an exposed port and makes a new port private.
	Visibility string `json:"visibility,omitempty"`
}

// PortsService exposes dock ports at URLs. Get it from Client.Ports.
type PortsService struct{ c *Client }

func portsPath(dockID string) string {
	return "/docks/" + url.PathEscape(dockID) + "/ports"
}

// List returns the dock's exposed ports.
func (s *PortsService) List(ctx context.Context, dockID string) ([]Port, error) {
	var out []Port
	if err := s.c.do(ctx, "GET", portsPath(dockID), nil, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Expose exposes a TCP port inside the dock. Exposing a port again returns the
// same URL; a non-empty params.Visibility then changes the port's visibility.
// Pass ExposePortParams{} for the default (private for a new port).
func (s *PortsService) Expose(ctx context.Context, dockID string, port int, params ExposePortParams) (*Port, error) {
	body := struct {
		Port int `json:"port"`
		ExposePortParams
	}{port, params}
	var out Port
	if err := s.c.do(ctx, "POST", portsPath(dockID), nil, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Remove unexposes a port; its URL stops resolving at once. It fails with code
// "port_not_found" when the port is not exposed.
func (s *PortsService) Remove(ctx context.Context, dockID string, port int) error {
	return s.c.do(ctx, "DELETE", portsPath(dockID)+"/"+strconv.Itoa(port), nil, nil, nil, nil)
}
