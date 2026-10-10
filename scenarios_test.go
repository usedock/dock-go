package dock_test

// Shared scenario runner. The scenarios live in usedock/dock (sdk-scenarios);
// see its README for the runner contract implemented here. Offline runs replay
// the recorded fixtures against a local fake server; live runs use a real
// control plane when DOCK_API_URL, DOCK_API_KEY and DOCK_WORKSPACE_ID are set.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"

	dock "github.com/usedock/dock-go"
)

type scenario struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Steps       []scenarioStep `yaml:"steps"`
	Cleanup     []cleanupStep  `yaml:"cleanup"`
}

type scenarioStep struct {
	ID      string            `yaml:"id"`
	Op      string            `yaml:"op"`
	Args    map[string]any    `yaml:"args"`
	Capture map[string]string `yaml:"capture"`
	Expect  map[string]any    `yaml:"expect"`
}

type cleanupStep struct {
	Op   string         `yaml:"op"`
	Args map[string]any `yaml:"args"`
}

type opsFile struct {
	HTTP      map[string]any `yaml:"http"`
	Composite map[string]any `yaml:"composite"`
}

type fixture struct {
	Scenario  string         `json:"scenario"`
	Variables map[string]any `json:"variables"`
	Exchanges []exchange     `json:"exchanges"`
}

type exchange struct {
	Step    string `json:"step"`
	Request struct {
		Method  string         `json:"method"`
		Path    string         `json:"path"`
		Headers map[string]any `json:"headers"`
		Body    any            `json:"body"`
	} `json:"request"`
	Response struct {
		Status int            `json:"status"`
		Body   map[string]any `json:"body"`
	} `json:"response"`
}

func scenariosDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("DOCK_SCENARIOS_DIR")
	if dir == "" {
		dir = filepath.Join("..", "dock", "sdk-scenarios")
	}
	if _, err := os.Stat(filepath.Join(dir, "ops.yaml")); err != nil {
		t.Skipf("scenario suite not found at %s (set DOCK_SCENARIOS_DIR to a checkout of usedock/dock sdk-scenarios)", dir)
	}
	return dir
}

func loadYAML(t *testing.T, path string, out any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(raw, out); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// jsonNorm round-trips v through JSON so numbers are float64 and maps are
// map[string]any, the same as decoded responses.
func jsonNorm(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

var varRE = regexp.MustCompile(`\$\{(\w+)\}`)
var wholeVarRE = regexp.MustCompile(`^\$\{(\w+)\}$`)

func substitute(v any, vars map[string]any) any {
	switch x := v.(type) {
	case string:
		if m := wholeVarRE.FindStringSubmatch(x); m != nil {
			return vars[m[1]]
		}
		return varRE.ReplaceAllStringFunc(x, func(s string) string {
			return fmt.Sprint(vars[varRE.FindStringSubmatch(s)[1]])
		})
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = substitute(e, vars)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = substitute(e, vars)
		}
		return out
	}
	return v
}

func missingVars(v any, vars map[string]any) []string {
	var missing []string
	for _, m := range varRE.FindAllStringSubmatch(fmt.Sprint(jsonNorm(v)), -1) {
		if _, ok := vars[m[1]]; !ok {
			missing = append(missing, m[1])
		}
	}
	return missing
}

func subsetMatch(want, got any) bool {
	if s, ok := want.(string); ok && s == "$any" {
		return got != nil
	}
	switch w := want.(type) {
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !subsetMatch(w[i], g[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, e := range w {
			if !subsetMatch(e, g[k]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(want, got)
}

// statusRecorder remembers the HTTP status of the last response, so the runner
// can report it without reaching into the SDK.
type statusRecorder struct {
	mu   sync.Mutex
	last int
}

func (s *statusRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(r)
	if err == nil {
		s.mu.Lock()
		s.last = resp.StatusCode
		s.mu.Unlock()
	}
	return resp, err
}

func (s *statusRecorder) status() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

type runner struct {
	t       *testing.T
	client  *dock.Client
	rec     *statusRecorder
	vars    map[string]any
	tracked []string
}

func newRunner(t *testing.T, baseURL, key string, vars map[string]any) *runner {
	rec := &statusRecorder{}
	c := dock.NewClient(dock.WithBaseURL(baseURL), dock.WithAPIKey(key), dock.WithHTTPClient(&http.Client{Transport: rec}))
	return &runner{t: t, client: c, rec: rec, vars: vars}
}

// decodeArgs maps op args onto an SDK params struct; an arg the struct does not
// know fails, which keeps the json tags aligned with ops.yaml.
func decodeArgs(args map[string]any, out any) error {
	b, err := json.Marshal(args)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func without(args map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for k, v := range args {
		out[k] = v
	}
	for _, k := range keys {
		delete(out, k)
	}
	return out
}

func str(args map[string]any, k string) string {
	s, _ := args[k].(string)
	return s
}

// call runs one op through the public SDK and returns the response envelope.
func (r *runner) call(ctx context.Context, op string, args map[string]any) map[string]any {
	r.t.Helper()
	id := str(args, "dockId")
	var (
		data any
		err  error
	)
	switch op {
	case "docks.create":
		var p dock.CreateParams
		if err = decodeArgs(args, &p); err == nil {
			var d *dock.Dock
			if d, err = r.client.Docks.Create(ctx, p); err == nil {
				data = d
				r.tracked = append(r.tracked, d.ID)
			}
		}
	case "docks.list":
		var p struct {
			Filter string `json:"filter"`
			All    bool   `json:"all"`
		}
		if err = decodeArgs(args, &p); err == nil {
			data, err = r.client.Docks.List(ctx, &dock.ListParams{Filter: p.Filter, All: p.All})
		}
	case "docks.get":
		data, err = r.client.Docks.Get(ctx, id)
	case "docks.update":
		var p dock.UpdateParams
		if err = decodeArgs(without(args, "dockId"), &p); err == nil {
			data, err = r.client.Docks.Update(ctx, id, p)
		}
	case "docks.exec":
		var p dock.ExecParams
		if err = decodeArgs(without(args, "dockId"), &p); err == nil {
			data, err = r.client.Docks.Exec(ctx, id, p)
		}
	case "docks.stop":
		var p dock.StopParams
		if err = decodeArgs(without(args, "dockId"), &p); err == nil {
			data, err = r.client.Docks.Stop(ctx, id, &p)
		}
	case "docks.resume":
		var p dock.ResumeParams
		if err = decodeArgs(without(args, "dockId"), &p); err == nil {
			data, err = r.client.Docks.Resume(ctx, id, &p)
		}
	case "docks.branch":
		var p dock.BranchParams
		if err = decodeArgs(without(args, "dockId"), &p); err == nil {
			var d *dock.Dock
			if d, err = r.client.Docks.Branch(ctx, id, &p); err == nil {
				data = d
				r.tracked = append(r.tracked, d.ID)
			}
		}
	case "docks.delete":
		var o *dock.Operation
		if o, err = r.client.Docks.Delete(ctx, id); err == nil {
			data = map[string]any{"operation": o}
		}
	case "operations.get":
		data, err = r.client.Operations.Get(ctx, str(args, "operationId"))
	case "docks.files.write":
		if err = r.client.Files.Write(ctx, id, str(args, "path"), []byte(str(args, "content"))); err == nil {
			data = map[string]any{"path": str(args, "path")}
		}
	case "docks.files.read":
		var b []byte
		if b, err = r.client.Files.Read(ctx, id, str(args, "path")); err == nil {
			data = map[string]any{"content": string(b)}
		}
	case "docks.files.list":
		var entries []string
		if entries, err = r.client.Files.List(ctx, id, str(args, "path")); err == nil {
			data = map[string]any{"entries": entries}
		}
	default:
		r.t.Fatalf("unknown op %s", op)
	}
	var apiErr *dock.Error
	switch {
	case errors.As(err, &apiErr):
		return map[string]any{"status": float64(apiErr.Status), "ok": false, "code": apiErr.Code, "message": apiErr.Message}
	case err != nil:
		r.t.Fatalf("%s: non-API error: %v", op, err)
	}
	return map[string]any{"status": float64(r.rec.status()), "ok": true, "data": jsonNorm(data)}
}

func capturePath(env map[string]any, path string) any {
	var cur any = env
	for _, k := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}

// run executes the steps, then cleanup, and reports the first failed assertion.
func (r *runner) run(sc scenario, live bool) {
	t := r.t
	ctx := context.Background()
	defer func() {
		for _, c := range sc.Cleanup {
			if len(missingVars(c.Args, r.vars)) > 0 {
				continue
			}
			args, _ := substitute(jsonNorm(c.Args), r.vars).(map[string]any)
			func() {
				defer func() { _ = recover() }()
				r.call(ctx, c.Op, args)
			}()
		}
		if live {
			for _, id := range r.tracked {
				_, _ = r.client.Docks.Delete(ctx, id)
			}
		}
	}()
	for _, st := range sc.Steps {
		args, _ := substitute(jsonNorm(st.Args), r.vars).(map[string]any)
		if args == nil {
			args = map[string]any{}
		}
		env := r.call(ctx, st.Op, args)
		for name, p := range st.Capture {
			r.vars[name] = capturePath(env, p)
		}
		want := jsonNorm(substitute(jsonNorm(st.Expect), r.vars)).(map[string]any)
		if env["status"] != want["status"] || env["ok"] != want["ok"] {
			t.Fatalf("step %s: got status=%v ok=%v code=%v message=%v, want status=%v ok=%v", st.ID, env["status"], env["ok"], env["code"], env["message"], want["status"], want["ok"])
		}
		if code, ok := want["code"]; ok && env["code"] != code {
			t.Fatalf("step %s: code %v, want %v", st.ID, env["code"], code)
		}
		if d, ok := want["data"]; ok && !subsetMatch(d, env["data"]) {
			got, _ := json.Marshal(env["data"])
			wantJSON, _ := json.Marshal(d)
			t.Fatalf("step %s: data %s does not contain %s", st.ID, got, wantJSON)
		}
	}
}

// fakeServer implements the README's recorded fake: first unused exchange with
// the same method and exact path whose body and headers are subsets.
func fakeServer(t *testing.T, fx fixture) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	used := make([]bool, len(fx.Exchanges))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body any = map[string]any{}
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
		}
		mu.Lock()
		defer mu.Unlock()
		for i, e := range fx.Exchanges {
			if used[i] || e.Request.Method != r.Method || e.Request.Path != r.URL.Path {
				continue
			}
			wantBody := e.Request.Body
			if wantBody == nil {
				wantBody = map[string]any{}
			}
			if !subsetMatch(wantBody, body) {
				continue
			}
			headersOK := true
			for h, v := range e.Request.Headers {
				if r.Header.Get(h) != fmt.Sprint(v) {
					headersOK = false
				}
			}
			if !headersOK {
				continue
			}
			used[i] = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(e.Response.Status)
			_ = json.NewEncoder(w).Encode(e.Response.Body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"ok":false,"code":"dock_not_found","message":"Dock not found"}`)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		var unused []string
		for i, u := range used {
			if !u {
				unused = append(unused, fx.Exchanges[i].Step)
			}
		}
		return unused
	}
}

func validateScenario(t *testing.T, file string, sc scenario, ops opsFile) {
	t.Helper()
	if sc.Name+".yaml" != file {
		t.Errorf("name %q does not match file %s", sc.Name, file)
	}
	if sc.Description == "" || len(sc.Steps) == 0 {
		t.Errorf("%s needs a description and steps", file)
	}
	known := func(op string) bool { _, h := ops.HTTP[op]; _, c := ops.Composite[op]; return h || c }
	ids := map[string]bool{}
	for _, st := range sc.Steps {
		if ids[st.ID] || st.ID == "" {
			t.Errorf("%s: step id %q empty or duplicated", file, st.ID)
		}
		ids[st.ID] = true
		if !known(st.Op) {
			t.Errorf("%s: unknown op %s", file, st.Op)
		}
		if _, ok := st.Expect["status"]; !ok {
			t.Errorf("%s step %s: expect.status missing", file, st.ID)
		}
		if _, ok := st.Expect["ok"]; !ok {
			t.Errorf("%s step %s: expect.ok missing", file, st.ID)
		}
	}
	for _, c := range sc.Cleanup {
		if !known(c.Op) {
			t.Errorf("%s: unknown cleanup op %s", file, c.Op)
		}
	}
}

func TestScenarios(t *testing.T) {
	dir := scenariosDir(t)
	var ops opsFile
	loadYAML(t, filepath.Join(dir, "ops.yaml"), &ops)
	files, err := filepath.Glob(filepath.Join(dir, "scenarios", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no scenarios in %s", dir)
	}
	sort.Strings(files)
	for _, path := range files {
		file := filepath.Base(path)
		var sc scenario
		loadYAML(t, path, &sc)
		name := strings.TrimSuffix(file, ".yaml")

		t.Run("offline/"+name, func(t *testing.T) {
			validateScenario(t, file, sc, ops)
			raw, err := os.ReadFile(filepath.Join(dir, "fixtures", name+".json"))
			if err != nil {
				t.Fatalf("fixture: %v", err)
			}
			var fx fixture
			if err := json.Unmarshal(raw, &fx); err != nil {
				t.Fatal(err)
			}
			srv, unused := fakeServer(t, fx)
			r := newRunner(t, srv.URL, "dock_fixture", fx.Variables)
			r.run(sc, false)
			if u := unused(); len(u) > 0 {
				t.Errorf("fixture exchanges never requested: %v", u)
			}
		})

		t.Run("live/"+name, func(t *testing.T) {
			url, key, ws := os.Getenv("DOCK_API_URL"), os.Getenv("DOCK_API_KEY"), os.Getenv("DOCK_WORKSPACE_ID")
			if url == "" || key == "" || ws == "" {
				t.Skip("set DOCK_API_URL, DOCK_API_KEY and DOCK_WORKSPACE_ID to run against a real Dock")
			}
			b := make([]byte, 4)
			_, _ = rand.Read(b)
			r := newRunner(t, url, key, map[string]any{"workspaceId": ws, "run": hex.EncodeToString(b)})
			r.run(sc, true)
		})
	}
}
