package dock

import (
	"context"
	"net/url"
)

// Connector is a workspace's connection to a tool (figma, linear, slack,
// sentry) or a read-only entry (github, model-keys). Tokens are never returned.
type Connector struct {
	// Kind is the connector name, for example "slack".
	Kind string `json:"kind"`
	// Type is "tool", "github" or "model-keys".
	Type string `json:"type,omitempty"`
	// Status is "connected", "not_connected" or "reauth_required".
	Status       string   `json:"status"`
	AccountLabel *string  `json:"accountLabel,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	// OAuthAvailable reports whether the kind can also be connected with OAuth.
	OAuthAvailable bool `json:"oauthAvailable,omitempty"`
	// ReadOnly entries cannot be connected or disconnected with Connect and
	// Disconnect.
	ReadOnly bool `json:"readOnly,omitempty"`
	// CredentialType is how the credential was obtained: "oauth",
	// "personal_token", "bot_token", "org_token", "user_token" or "token". Nil
	// when not connected or not recorded.
	CredentialType *string `json:"credentialType,omitempty"`
	UpdatedAt      *string `json:"updatedAt,omitempty"`
}

// ConnectorsService is the workspace connectors API. Get it from
// Client.Connectors. Attach connectors to a dock with CreateParams.Connectors.
type ConnectorsService struct{ c *Client }

func connectorsPath(workspaceID string) string {
	return "/workspaces/" + url.PathEscape(workspaceID) + "/connectors"
}

// List returns every connector of a workspace, connected or not.
func (s *ConnectorsService) List(ctx context.Context, workspaceID string) ([]Connector, error) {
	var out []Connector
	if err := s.c.do(ctx, "GET", connectorsPath(workspaceID), nil, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Connect stores a personal or bot token for kind after the API checks it with
// the vendor. Only organization admins can connect.
func (s *ConnectorsService) Connect(ctx context.Context, workspaceID, kind, token string) (*Connector, error) {
	var out Connector
	body := struct {
		Token string `json:"token"`
	}{token}
	if err := s.c.do(ctx, "PUT", connectorsPath(workspaceID)+"/"+url.PathEscape(kind), nil, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Disconnect removes the connection for kind. The result has Kind and a
// Status of "not_connected". Only organization admins can disconnect.
func (s *ConnectorsService) Disconnect(ctx context.Context, workspaceID, kind string) (*Connector, error) {
	var out Connector
	if err := s.c.do(ctx, "DELETE", connectorsPath(workspaceID)+"/"+url.PathEscape(kind), nil, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
