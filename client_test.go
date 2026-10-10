package dock_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dock "github.com/usedock/dock-go"
)

type seen struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   map[string]any
}

// serve answers every request with status and body, recording the last one.
func serve(t *testing.T, status int, body string) (*dock.Client, *seen) {
	t.Helper()
	got := &seen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.Method, got.Path, got.Query, got.Header = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone()
		got.Body = nil
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &got.Body); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return dock.NewClient(dock.WithAPIKey("dock_test"), dock.WithBaseURL(srv.URL)), got
}

const dockJSON = `{"ok":true,"data":{"id":"d1","name":"n","state":"ready","cpus":2,"extra":{"unknown":true}}}`

func TestCreate(t *testing.T) {
	c, got := serve(t, 201, dockJSON)
	d, err := c.Docks.Create(context.Background(), dock.CreateParams{WorkspaceID: "w", Name: "n", CPUs: 2, IdempotencyKey: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "d1" || d.Name != "n" || d.State != "ready" || d.CPUs != 2 {
		t.Fatalf("dock %+v", d)
	}
	if got.Method != "POST" || got.Path != "/v1/docks" {
		t.Fatalf("%s %s", got.Method, got.Path)
	}
	if got.Header.Get("Authorization") != "Bearer dock_test" || got.Header.Get("Idempotency-Key") != "k1" {
		t.Fatalf("headers %v", got.Header)
	}
	if got.Body["workspaceId"] != "w" || got.Body["cpus"] != float64(2) {
		t.Fatalf("body %v", got.Body)
	}
	if _, ok := got.Body["memoryMiB"]; ok {
		t.Fatalf("zero fields must be omitted: %v", got.Body)
	}
	if _, ok := got.Body["IdempotencyKey"]; ok {
		t.Fatalf("idempotency key leaked into body: %v", got.Body)
	}
}

func TestListAndFilters(t *testing.T) {
	c, got := serve(t, 200, `{"ok":true,"data":[{"id":"a","name":"x","state":"ready"},{"id":"b","name":"y","state":"archived"}]}`)
	ds, err := c.Docks.List(context.Background(), nil)
	if err != nil || len(ds) != 2 || ds[1].State != "archived" {
		t.Fatalf("%v %+v", err, ds)
	}
	if got.Method != "GET" || got.Path != "/v1/docks" || got.Query != "" {
		t.Fatalf("%s %s?%s", got.Method, got.Path, got.Query)
	}
	if _, err := c.Docks.List(context.Background(), &dock.ListParams{Filter: "web", All: true}); err != nil {
		t.Fatal(err)
	}
	if got.Query != "all=true&filter=web" {
		t.Fatalf("query %q", got.Query)
	}
}

func TestGetUpdate(t *testing.T) {
	c, got := serve(t, 200, dockJSON)
	if _, err := c.Docks.Get(context.Background(), "d1"); err != nil {
		t.Fatal(err)
	}
	if got.Method != "GET" || got.Path != "/v1/docks/d1" {
		t.Fatalf("%s %s", got.Method, got.Path)
	}
	if _, err := c.Docks.Update(context.Background(), "d1", dock.UpdateParams{Name: "new"}); err != nil {
		t.Fatal(err)
	}
	if got.Method != "PATCH" || got.Path != "/v1/docks/d1" || got.Body["name"] != "new" {
		t.Fatalf("%s %s %v", got.Method, got.Path, got.Body)
	}
}

func TestExec(t *testing.T) {
	c, got := serve(t, 200, `{"ok":true,"data":{"exitCode":3,"stdout":"a\n","stderr":"b\n","truncated":false,"durationMs":5}}`)
	r, err := c.Docks.Exec(context.Background(), "d1", dock.ExecParams{Command: "echo a", Cwd: "/tmp", Env: map[string]string{"A": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 3 || r.Stdout != "a\n" || r.Stderr != "b\n" {
		t.Fatalf("%+v", r)
	}
	if got.Method != "POST" || got.Path != "/v1/docks/d1/exec" || got.Body["command"] != "echo a" || got.Body["cwd"] != "/tmp" {
		t.Fatalf("%s %s %v", got.Method, got.Path, got.Body)
	}
}

func TestStopResumeBranch(t *testing.T) {
	c, got := serve(t, 202, `{"ok":true,"data":{"id":"d1","state":"archived"}}`)
	ctx := context.Background()
	d, err := c.Docks.Stop(ctx, "d1", nil)
	if err != nil || d.State != "archived" {
		t.Fatalf("%v %+v", err, d)
	}
	if got.Method != "POST" || got.Path != "/v1/docks/d1/stop" || got.Body == nil {
		t.Fatalf("%s %s %v", got.Method, got.Path, got.Body)
	}
	if _, err := c.Docks.Stop(ctx, "d1", &dock.StopParams{Force: true}); err != nil || got.Body["force"] != true {
		t.Fatalf("%v %v", err, got.Body)
	}
	if _, err := c.Docks.Resume(ctx, "d1", &dock.ResumeParams{NoEnv: true}); err != nil {
		t.Fatal(err)
	}
	if got.Path != "/v1/docks/d1/resume" || got.Body["noEnv"] != true {
		t.Fatalf("%s %v", got.Path, got.Body)
	}
	if _, err := c.Docks.Branch(ctx, "d1", &dock.BranchParams{Name: "b", IdempotencyKey: "k"}); err != nil {
		t.Fatal(err)
	}
	if got.Path != "/v1/docks/d1/branch" || got.Body["name"] != "b" || got.Header.Get("Idempotency-Key") != "k" {
		t.Fatalf("%s %v %v", got.Path, got.Body, got.Header)
	}
	if _, err := c.Docks.Branch(ctx, "d1", nil); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteSendsConfirmHeader(t *testing.T) {
	c, got := serve(t, 202, `{"ok":true,"data":{"operation":{"id":"op1","targetType":"dock","targetId":"d1","status":"completed"}}}`)
	op, err := c.Docks.Delete(context.Background(), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "DELETE" || got.Path != "/v1/docks/d1" || got.Header.Get("X-Confirm-Delete") != "d1" {
		t.Fatalf("%s %s %v", got.Method, got.Path, got.Header)
	}
	if op.ID != "op1" || op.TargetID != "d1" || op.Status != "completed" {
		t.Fatalf("%+v", op)
	}
}

func TestOperationsGet(t *testing.T) {
	for name, body := range map[string]string{
		"bare":    `{"ok":true,"data":{"id":"op1","status":"running"}}`,
		"wrapped": `{"ok":true,"data":{"operation":{"id":"op1","status":"running"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, got := serve(t, 200, body)
			op, err := c.Operations.Get(context.Background(), "op1")
			if err != nil || op.ID != "op1" || op.Status != "running" {
				t.Fatalf("%v %+v", err, op)
			}
			if got.Method != "GET" || got.Path != "/v1/operations/op1" {
				t.Fatalf("%s %s", got.Method, got.Path)
			}
		})
	}
}

func execBody(exit int, stdout string) string {
	b, _ := json.Marshal(map[string]any{"ok": true, "data": map[string]any{"exitCode": exit, "stdout": stdout, "stderr": "boom\n", "truncated": false}})
	return string(b)
}

func TestFilesWrite(t *testing.T) {
	c, got := serve(t, 200, execBody(0, ""))
	if err := c.Files.Write(context.Background(), "d1", "/a/it's.txt", []byte("hi\n")); err != nil {
		t.Fatal(err)
	}
	want := "mkdir -p -- \"$(dirname -- '/a/it'\\''s.txt')\" && printf '%s' '" + base64.StdEncoding.EncodeToString([]byte("hi\n")) + "' | base64 -d > '/a/it'\\''s.txt'"
	if got.Path != "/v1/docks/d1/exec" || got.Body["command"] != want {
		t.Fatalf("command %q", got.Body["command"])
	}
}

func TestFilesWriteTooLargeSendsNothing(t *testing.T) {
	c, got := serve(t, 200, execBody(0, ""))
	err := c.Files.Write(context.Background(), "d1", "/a", make([]byte, 49*1024))
	var apiErr *dock.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "file_too_large" {
		t.Fatalf("err %v", err)
	}
	if got.Method != "" {
		t.Fatal("a request was sent")
	}
	if err := c.Files.Write(context.Background(), "d1", "/a", make([]byte, 40*1024)); err != nil {
		t.Fatalf("40 KB should fit: %v", err)
	}
}

func TestFilesReadAndList(t *testing.T) {
	c, got := serve(t, 200, execBody(0, base64.StdEncoding.EncodeToString([]byte("héllo\n"))))
	b, err := c.Files.Read(context.Background(), "d1", "/x y")
	if err != nil || string(b) != "héllo\n" {
		t.Fatalf("%v %q", err, b)
	}
	if got.Body["command"] != "base64 -w0 -- '/x y'" {
		t.Fatalf("command %q", got.Body["command"])
	}
	c, got = serve(t, 200, execBody(0, "a.txt\n\n.hidden\nb\n"))
	entries, err := c.Files.List(context.Background(), "d1", "/w")
	if err != nil || strings.Join(entries, ",") != "a.txt,.hidden,b" {
		t.Fatalf("%v %v", err, entries)
	}
	if got.Body["command"] != "ls -1A -- '/w'" {
		t.Fatalf("command %q", got.Body["command"])
	}
}

func TestFilesNonZeroExitIsError(t *testing.T) {
	c, _ := serve(t, 200, execBody(1, ""))
	_, err := c.Files.Read(context.Background(), "d1", "/missing")
	var apiErr *dock.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "file_command_failed" || !strings.Contains(apiErr.Message, "boom") {
		t.Fatalf("err %v", err)
	}
	if err := c.Files.Write(context.Background(), "d1", "", nil); err == nil {
		t.Fatal("empty path must fail")
	}
}

func TestErrorEnvelope(t *testing.T) {
	c, _ := serve(t, 404, `{"ok":false,"code":"dock_not_found","message":"Dock not found"}`)
	_, err := c.Docks.Get(context.Background(), "x")
	var apiErr *dock.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err %v", err)
	}
	if apiErr.Code != "dock_not_found" || apiErr.Status != 404 || apiErr.Message != "Dock not found" {
		t.Fatalf("%+v", apiErr)
	}
	if !strings.Contains(err.Error(), "dock_not_found") {
		t.Fatalf("Error() %q", err.Error())
	}
}

func TestNonEnvelopeResponses(t *testing.T) {
	c, _ := serve(t, 502, `<html>bad gateway</html>`)
	_, err := c.Docks.Get(context.Background(), "x")
	var apiErr *dock.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "http_502" || apiErr.Status != 502 {
		t.Fatalf("err %v", err)
	}
	c, _ = serve(t, 500, `{"ok":false}`)
	_, err = c.Docks.Get(context.Background(), "x")
	if !errors.As(err, &apiErr) || apiErr.Code != "http_500" {
		t.Fatalf("err %v", err)
	}
	c, _ = serve(t, 200, `not json`)
	_, err = c.Docks.Get(context.Background(), "x")
	if !errors.As(err, &apiErr) || apiErr.Code != "bad_response" {
		t.Fatalf("err %v", err)
	}
}

func TestEnvDefaultsAndOverrides(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_, _ = io.WriteString(w, dockJSON)
	}))
	defer srv.Close()
	t.Setenv("DOCK_API_KEY", "dock_env")
	t.Setenv("DOCK_API_URL", srv.URL+"/")
	if _, err := dock.NewClient().Docks.Get(context.Background(), "d1"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer dock_env" || gotPath != "/v1/docks/d1" {
		t.Fatalf("%q %q", gotAuth, gotPath)
	}
	if _, err := dock.NewClient(dock.WithAPIKey("dock_opt")).Docks.Get(context.Background(), "d1"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer dock_opt" {
		t.Fatalf("%q", gotAuth)
	}
}

func TestMissingAPIKey(t *testing.T) {
	t.Setenv("DOCK_API_KEY", "")
	_, err := dock.NewClient(dock.WithBaseURL("http://127.0.0.1:1")).Docks.Get(context.Background(), "d1")
	var apiErr *dock.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "missing_api_key" {
		t.Fatalf("err %v", err)
	}
}

func TestCustomHTTPClientAndContext(t *testing.T) {
	used := false
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := dock.NewClient(dock.WithAPIKey("dock_secret"), dock.WithHTTPClient(hc), dock.WithBaseURL("http://example.invalid"))
	_, err := c.Docks.Get(ctx, "d1")
	if err == nil || !used {
		t.Fatalf("err %v used %v", err, used)
	}
	if strings.Contains(err.Error(), "dock_secret") {
		t.Fatalf("key leaked: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
