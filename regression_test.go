package dock_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	dock "github.com/usedock/dock-go"
)

func truncatedExecBody(stdout string) string {
	b, _ := json.Marshal(map[string]any{"ok": true, "data": map[string]any{"exitCode": 0, "stdout": stdout, "stderr": "", "truncated": true}})
	return string(b)
}

// The API clips stdout to its first 1 MiB and sets truncated. 1 MiB is a
// multiple of 4, so a clipped base64 stream still decodes: Read must not
// return the first 768 KiB as if it were the whole file.
func TestFilesReadRefusesTruncatedOutput(t *testing.T) {
	c, _ := serve(t, 200, truncatedExecBody(base64.StdEncoding.EncodeToString(make([]byte, 786432))))
	b, err := c.Files.Read(context.Background(), "d1", "/big")
	var apiErr *dock.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "output_truncated" {
		t.Fatalf("err %v, got %d bytes", err, len(b))
	}
	c, _ = serve(t, 200, truncatedExecBody("a\nb\n"))
	if _, err := c.Files.List(context.Background(), "d1", "/w"); !errors.As(err, &apiErr) || apiErr.Code != "output_truncated" {
		t.Fatalf("list err %v", err)
	}
}

func TestTransportErrorKeepsContextCause(t *testing.T) {
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	c := dock.NewClient(dock.WithAPIKey("dock_secret"), dock.WithHTTPClient(hc), dock.WithBaseURL("http://example.invalid"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := c.Docks.Get(ctx, "d1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(err, DeadlineExceeded) = false: %v", err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if _, err := c.Docks.Get(ctx2, "d1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(err, Canceled) = false: %v", err)
	}
	if strings.Contains(err.Error(), "dock_secret") {
		t.Fatalf("key leaked: %v", err)
	}
}

func TestClientFormattingHidesKey(t *testing.T) {
	c := dock.NewClient(dock.WithAPIKey("dock_secret"), dock.WithBaseURL("http://example.invalid"))
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		if s := fmt.Sprintf(verb, c); strings.Contains(s, "dock_secret") {
			t.Fatalf("%s of *Client leaks the key: %s", verb, s)
		}
		if s := fmt.Sprintf(verb, *c); strings.Contains(s, "dock_secret") {
			t.Fatalf("%s of Client leaks the key: %s", verb, s)
		}
	}
}

func TestUpdateClearTTLSendsNull(t *testing.T) {
	c, got := serve(t, 200, dockJSON)
	if _, err := c.Docks.Update(context.Background(), "d1", dock.UpdateParams{ClearTTL: true}); err != nil {
		t.Fatal(err)
	}
	v, ok := got.Body["ttlSeconds"]
	if !ok || v != nil {
		t.Fatalf("ttlSeconds must be sent as null: %v", got.Body)
	}
	if _, err := c.Docks.Update(context.Background(), "d1", dock.UpdateParams{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Body["ttlSeconds"]; ok {
		t.Fatalf("ttlSeconds must be omitted by default: %v", got.Body)
	}
	if _, err := c.Docks.Update(context.Background(), "d1", dock.UpdateParams{TTLSeconds: 120, ClearTTL: true}); err == nil {
		t.Fatal("TTLSeconds with ClearTTL must be refused")
	}
}
