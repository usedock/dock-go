package dock

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
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

// Runs the real command templates in bash: hostile names must be written,
// read and listed as plain file names, never executed.
func TestFileCommandsInBash(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	if exec.Command(bash, "-c", "base64 -w0 </dev/null").Run() != nil {
		t.Skip("GNU base64 not available")
	}
	for _, name := range []string{"it's.txt", "-n.txt", "dir with space/$(touch pwned).txt", "x'; touch pwned; echo '"} {
		dir := t.TempDir()
		content := "line 1\n'quoted' $HOME `x`\n"
		q := shellWord(name)
		write := "mkdir -p -- \"$(dirname -- " + q + ")\" && printf '%s' '" + base64.StdEncoding.EncodeToString([]byte(content)) + "' | base64 -d > " + q
		run := func(cmd string) string {
			c := exec.Command(bash, "-c", cmd)
			c.Dir = dir
			out, err := c.CombinedOutput()
			if err != nil {
				t.Fatalf("%q: %v: %s", name, err, out)
			}
			return string(out)
		}
		run(write)
		got, _ := base64.StdEncoding.DecodeString(run("base64 -w0 -- " + q))
		if string(got) != content {
			t.Errorf("%q: read back %q", name, got)
		}
		if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
			t.Errorf("%q: command injection ran", name)
		}
	}
}
