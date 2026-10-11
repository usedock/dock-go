package dock

import (
	"context"
	"net/url"
)

// ProviderKeyMode is how an environment hands the organization's provider keys
// to docks.
type ProviderKeyMode string

const (
	// ProviderKeysGateway (the default) keeps provider keys in the control plane;
	// the dock gets a placeholder key plus gateway base URLs.
	ProviderKeysGateway ProviderKeyMode = "gateway"
	// ProviderKeysPlain writes the real key into the dock, so anyone who can run a
	// command in it can read the key. Weaker; admins only.
	ProviderKeysPlain ProviderKeyMode = "plain"
)

// Environment is a versioned bundle of variables, files and posture applied to
// docks.
type Environment struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	IsDefault      bool   `json:"isDefault"`
	CurrentVersion int    `json:"currentVersion"`
	// SecretsAccess is "members" or "admins".
	SecretsAccess         string `json:"secretsAccess"`
	SafeForThirdParties   bool   `json:"safeForThirdParties"`
	PassGithub            bool   `json:"passGithub"`
	PassSecrets           bool   `json:"passSecrets"`
	PassDockCredentials   bool   `json:"passDockCredentials"`
	PassAgentsCredentials bool   `json:"passAgentsCredentials"`
	// ProviderKeys is ProviderKeysGateway or ProviderKeysPlain.
	ProviderKeys ProviderKeyMode `json:"providerKeys"`
	// EgressAllow lists the HTTPS hosts docks may reach; nil leaves egress open.
	EgressAllow []string `json:"egressAllow"`
	VarNames    []string `json:"varNames,omitempty"`
	CreatedAt   string   `json:"createdAt,omitempty"`
	UpdatedAt   string   `json:"updatedAt,omitempty"`
}

// UpdateEnvironmentParams are the fields of Environments.Update; nil pointers
// and empty strings are left unchanged. Posture changes create a new version.
type UpdateEnvironmentParams struct {
	Name                  string          `json:"name,omitempty"`
	IsDefault             *bool           `json:"isDefault,omitempty"`
	SecretsAccess         string          `json:"secretsAccess,omitempty"`
	SafeForThirdParties   *bool           `json:"safeForThirdParties,omitempty"`
	PassGithub            *bool           `json:"passGithub,omitempty"`
	PassSecrets           *bool           `json:"passSecrets,omitempty"`
	PassDockCredentials   *bool           `json:"passDockCredentials,omitempty"`
	PassAgentsCredentials *bool           `json:"passAgentsCredentials,omitempty"`
	ProviderKeys          ProviderKeyMode `json:"providerKeys,omitempty"`
}

// EnvironmentsService is the environments API. Get it from Client.Environments.
type EnvironmentsService struct{ c *Client }

func orgQuery(organizationID string) url.Values {
	if organizationID == "" {
		return nil
	}
	return url.Values{"organizationId": {organizationID}}
}

// List returns an organization's environments.
func (s *EnvironmentsService) List(ctx context.Context, organizationID string) ([]Environment, error) {
	var out []Environment
	if err := s.c.do(ctx, "GET", "/environments", orgQuery(organizationID), nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get returns an environment by id or name. organizationID is needed only when
// ref is a name.
func (s *EnvironmentsService) Get(ctx context.Context, ref, organizationID string) (*Environment, error) {
	var out Environment
	if err := s.c.do(ctx, "GET", "/environments/"+url.PathEscape(ref), orgQuery(organizationID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update changes an environment's posture, including ProviderKeys
// (ProviderKeysPlain needs an organization admin). organizationID is needed
// only when ref is a name.
func (s *EnvironmentsService) Update(ctx context.Context, ref, organizationID string, p UpdateEnvironmentParams) (*Environment, error) {
	var out Environment
	if err := s.c.do(ctx, "PATCH", "/environments/"+url.PathEscape(ref), orgQuery(organizationID), nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
