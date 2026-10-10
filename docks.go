package dock

import (
	"context"
	"encoding/json"
	"net/url"
)

// Dock is an isolated Linux machine. Only ID, Name and State are relied upon;
// other fields are filled when the API returns them.
type Dock struct {
	ID              string   `json:"id"`
	WorkspaceID     string   `json:"workspaceId,omitempty"`
	Name            string   `json:"name"`
	State           string   `json:"state"`
	Image           string   `json:"image,omitempty"`
	Runtime         string   `json:"runtime,omitempty"`
	CPUs            int      `json:"cpus,omitempty"`
	MemoryMiB       int      `json:"memoryMiB,omitempty"`
	DiskGiB         int      `json:"diskGiB,omitempty"`
	PersistentPaths []string `json:"persistentPaths,omitempty"`
}

// CreateParams are the fields of Docks.Create. Zero values are omitted, so the
// API defaults apply.
type CreateParams struct {
	WorkspaceID string            `json:"workspaceId"`
	Name        string            `json:"name,omitempty"`
	Image       string            `json:"image,omitempty"`
	CPUs        int               `json:"cpus,omitempty"`
	MemoryMiB   int               `json:"memoryMiB,omitempty"`
	DiskGiB     int               `json:"diskGiB,omitempty"`
	TTLSeconds  int               `json:"ttlSeconds,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	// IdempotencyKey makes a retry return the original dock. Sent as the
	// idempotency-key header.
	IdempotencyKey string `json:"-"`
}

// ListParams filters Docks.List. A nil value lists the default set.
type ListParams struct {
	Filter string
	All    bool
}

// UpdateParams are the fields of Docks.Update; zero values are left unchanged.
type UpdateParams struct {
	Name       string `json:"name,omitempty"`
	CPUs       int    `json:"cpus,omitempty"`
	MemoryMiB  int    `json:"memoryMiB,omitempty"`
	DiskGiB    int    `json:"diskGiB,omitempty"`
	TTLSeconds int    `json:"ttlSeconds,omitempty"`
	// ClearTTL turns auto-stop off by sending ttlSeconds as null. It cannot be
	// combined with TTLSeconds.
	ClearTTL bool `json:"-"`
}

// MarshalJSON sends ttlSeconds as null when ClearTTL is set.
func (p UpdateParams) MarshalJSON() ([]byte, error) {
	type plain UpdateParams
	if !p.ClearTTL {
		return json.Marshal(plain(p))
	}
	return json.Marshal(struct {
		plain
		TTLSeconds *int `json:"ttlSeconds"`
	}{plain: plain(p)})
}

// ExecParams are the fields of Docks.Exec.
type ExecParams struct {
	Command        string            `json:"command"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
}

// ExecResult is the outcome of a command. A non-zero ExitCode is not an error.
type ExecResult struct {
	ExitCode   int    `json:"exitCode"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	Truncated  bool   `json:"truncated"`
	DurationMs int    `json:"durationMs,omitempty"`
}

// StopParams are the optional fields of Docks.Stop.
type StopParams struct {
	Force bool `json:"force,omitempty"`
}

// ResumeParams are the optional fields of Docks.Resume.
type ResumeParams struct {
	CPUs        int    `json:"cpus,omitempty"`
	MemoryMiB   int    `json:"memoryMiB,omitempty"`
	DiskGiB     int    `json:"diskGiB,omitempty"`
	TTLSeconds  int    `json:"ttlSeconds,omitempty"`
	Environment string `json:"environment,omitempty"`
	NoEnv       bool   `json:"noEnv,omitempty"`
}

// BranchParams are the optional fields of Docks.Branch.
type BranchParams struct {
	Name           string            `json:"name,omitempty"`
	Image          string            `json:"image,omitempty"`
	CPUs           int               `json:"cpus,omitempty"`
	MemoryMiB      int               `json:"memoryMiB,omitempty"`
	DiskGiB        int               `json:"diskGiB,omitempty"`
	TTLSeconds     int               `json:"ttlSeconds,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Environment    string            `json:"environment,omitempty"`
	NoEnv          bool              `json:"noEnv,omitempty"`
	IdempotencyKey string            `json:"-"`
}

// DocksService is the docks API. Get it from Client.Docks.
type DocksService struct{ c *Client }

func dockPath(id string) string { return "/docks/" + url.PathEscape(id) }

func idemHeader(key string) map[string]string {
	if key == "" {
		return nil
	}
	return map[string]string{"idempotency-key": key}
}

// Create makes a dock and returns once it is ready.
func (s *DocksService) Create(ctx context.Context, p CreateParams) (*Dock, error) {
	var d Dock
	if err := s.c.do(ctx, "POST", "/docks", nil, idemHeader(p.IdempotencyKey), p, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// List returns docks. params may be nil.
func (s *DocksService) List(ctx context.Context, params *ListParams) ([]Dock, error) {
	q := url.Values{}
	if params != nil {
		if params.Filter != "" {
			q.Set("filter", params.Filter)
		}
		if params.All {
			q.Set("all", "true")
		}
	}
	var ds []Dock
	if err := s.c.do(ctx, "GET", "/docks", q, nil, nil, &ds); err != nil {
		return nil, err
	}
	return ds, nil
}

// Get returns one dock.
func (s *DocksService) Get(ctx context.Context, dockID string) (*Dock, error) {
	var d Dock
	if err := s.c.do(ctx, "GET", dockPath(dockID), nil, nil, nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// Update renames or resizes a dock, or changes its auto-stop TTL.
func (s *DocksService) Update(ctx context.Context, dockID string, p UpdateParams) (*Dock, error) {
	if p.ClearTTL && p.TTLSeconds != 0 {
		return nil, &Error{Code: "invalid_argument", Message: "set TTLSeconds or ClearTTL, not both"}
	}
	var d Dock
	if err := s.c.do(ctx, "PATCH", dockPath(dockID), nil, nil, p, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// Exec runs a shell command in a running dock.
func (s *DocksService) Exec(ctx context.Context, dockID string, p ExecParams) (*ExecResult, error) {
	var r ExecResult
	if err := s.c.do(ctx, "POST", dockPath(dockID)+"/exec", nil, nil, p, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Stop stops a dock, keeping its state, and returns once it is stopped.
// The API answers with only the id and state, so the other Dock fields are
// zero; use Get for the full dock. params may be nil.
func (s *DocksService) Stop(ctx context.Context, dockID string, params *StopParams) (*Dock, error) {
	var body any = struct{}{}
	if params != nil {
		body = params
	}
	return s.action(ctx, dockID, "/stop", nil, body)
}

// Resume starts a stopped dock and returns once it is ready. Like Stop, only
// ID and State are filled. params may be nil.
func (s *DocksService) Resume(ctx context.Context, dockID string, params *ResumeParams) (*Dock, error) {
	var body any = struct{}{}
	if params != nil {
		body = params
	}
	return s.action(ctx, dockID, "/resume", nil, body)
}

// Branch creates a new dock from the state of dockID. params may be nil.
func (s *DocksService) Branch(ctx context.Context, dockID string, params *BranchParams) (*Dock, error) {
	var body any = struct{}{}
	var h map[string]string
	if params != nil {
		body = params
		h = idemHeader(params.IdempotencyKey)
	}
	return s.action(ctx, dockID, "/branch", h, body)
}

func (s *DocksService) action(ctx context.Context, dockID, suffix string, headers map[string]string, body any) (*Dock, error) {
	var d Dock
	if err := s.c.do(ctx, "POST", dockPath(dockID)+suffix, nil, headers, body, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// Delete deletes a dock. The SDK sends the required x-confirm-delete header.
// The returned operation can be polled with Operations.Get.
func (s *DocksService) Delete(ctx context.Context, dockID string) (*Operation, error) {
	var out struct {
		Operation Operation `json:"operation"`
	}
	h := map[string]string{"x-confirm-delete": dockID}
	if err := s.c.do(ctx, "DELETE", dockPath(dockID), nil, h, nil, &out); err != nil {
		return nil, err
	}
	return &out.Operation, nil
}
