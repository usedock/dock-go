package dock

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellWord(t *testing.T) {
	cases := map[string]string{
		"/a/b":       `'/a/b'`,
		"it's.txt":   `'it'\''s.txt'`,
		"a b":        `'a b'`,
		"-n.txt":     `'-n.txt'`,
		"$(touch x)": `'$(touch x)'`,
	}
	for in, want := range cases {
		if got := shellWord(in); got != want {
			t.Errorf("shellWord(%q) = %s, want %s", in, got, want)
		}
	}
}

// The rendered commands must equal the recorded sdk-scenarios fixture
// requests, which come from the ops.yaml templates.
func TestCommandsMatchFixtures(t *testing.T) {
	dir := os.Getenv("DOCK_SCENARIOS_DIR")
	if dir == "" {
		dir = filepath.Join("..", "dock", "sdk-scenarios")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "fixtures", "files-roundtrip.json"))
	if err != nil {
		t.Skipf("scenario fixtures not found at %s", dir)
	}
	var fx struct {
		Exchanges []struct {
			Step    string `json:"step"`
			Request struct {
				Body struct {
					Command string `json:"command"`
				} `json:"body"`
			} `json:"request"`
		} `json:"exchanges"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	const p = "/home/user/work/notes/hello.txt"
	want := map[string]string{
		"write": writeCommand(p, []byte("hello from dock\nsecond line\n")),
		"read":  readCommand(p),
		"list":  listCommand("/home/user/work/notes"),
	}
	seen := 0
	for _, ex := range fx.Exchanges {
		w, ok := want[ex.Step]
		if !ok {
			continue
		}
		seen++
		if ex.Request.Body.Command != w {
			t.Errorf("%s command\n got %s\nwant %s", ex.Step, w, ex.Request.Body.Command)
		}
	}
	if seen != len(want) {
		t.Errorf("fixture has %d of %d file steps", seen, len(want))
	}
}

// Runs the real command templates in bash: hostile names must be written,
// read and listed as plain file names, never executed.
func TestFileCommandsInBash(t *testing.T) {
	bash, pathPrefix, reason := resolveBash(runtime.GOOS, os.Getenv, fileExists, exec.LookPath)
	// CI must run this test, never skip it silently.
	skip := func(why string) {
		if os.Getenv("CI") != "" {
			t.Fatal(why)
		}
		t.Skip(why)
	}
	if bash == "" {
		skip(reason)
	}
	env := os.Environ()
	if pathPrefix != "" {
		env = append(env, "PATH="+pathPrefix+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	probe := func(cmd string) error {
		c := exec.Command(bash, "-c", cmd)
		c.Env = env
		return c.Run()
	}
	if probe("base64 -w0 </dev/null") != nil {
		skip("GNU base64 not available")
	}
	if probe("find -H . -maxdepth 0") != nil {
		skip("GNU find not available")
	}
	runIn := func(t *testing.T, dir, cmd string) (string, error) {
		c := exec.Command(bash, "-c", cmd)
		c.Env = env
		c.Dir = dir
		out, err := c.CombinedOutput()
		return string(out), err
	}
	decodeLines := func(t *testing.T, out string) []string {
		var names []string
		for _, l := range strings.Split(out, "\n") {
			if l == "" {
				continue
			}
			b, err := base64.StdEncoding.DecodeString(l)
			if err != nil {
				t.Fatalf("bad line %q", l)
			}
			names = append(names, string(b))
		}
		return names
	}
	// Each name is a file inside its own directory "d"; a name with a slash
	// also exercises dirname and mkdir -p.
	for _, name := range []string{
		"it's.txt", "-n.txt", "dir with space/$(touch pwned).txt", "x'; touch pwned; echo '",
		"new\nline.txt", "trailing-newline\n", "bang!.txt", `dq"quote.txt`, "-rf/-x", "back`tick`.txt",
	} {
		dir := t.TempDir()
		content := "line 1\n'quoted' $HOME `x`\n"
		if out, err := runIn(t, dir, writeCommand(name, []byte(content))); err != nil {
			t.Fatalf("%q write: %v: %s", name, err, out)
		}
		out, err := runIn(t, dir, readCommand(name))
		if err != nil {
			t.Fatalf("%q read: %v: %s", name, err, out)
		}
		if got, _ := base64.StdEncoding.DecodeString(out); string(got) != content {
			t.Errorf("%q: read back %q", name, got)
		}
		if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
			t.Errorf("%q: command injection ran", name)
		}
	}
	// List a relative directory whose entries are hostile names.
	dir := t.TempDir()
	hostile := []string{"new\nline", "-n", "bang!", `dq"x`, "it's", ".hidden", "trail\n"}
	for _, n := range hostile {
		if out, err := runIn(t, dir, writeCommand("-d/"+n, []byte("x"))); err != nil {
			t.Fatalf("%q write: %v: %s", n, err, out)
		}
	}
	out, err := runIn(t, dir, listCommand("-d"))
	if err != nil {
		t.Fatalf("list: %v: %s", err, out)
	}
	got := decodeLines(t, out)
	if len(got) != len(hostile) {
		t.Fatalf("list gave %q, want %d entries", got, len(hostile))
	}
	have := map[string]bool{}
	for _, g := range got {
		have[g] = true
	}
	for _, n := range hostile {
		if !have[n] {
			t.Errorf("list missing %q in %q", n, got)
		}
	}
	// A symlink to a directory lists the target's entries (find -H).
	if out, err := runIn(t, dir, "ln -s -- -d link && "+listCommand("link")); err != nil || len(decodeLines(t, out)) != len(hostile) {
		t.Errorf("symlinked dir: %v: %q", err, out)
	}
	// A path that is not a directory exits non-zero.
	if _, err := runIn(t, dir, listCommand("nope")); err == nil {
		t.Error("listing a missing directory must fail")
	}
}
