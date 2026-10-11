package dock

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
)

// On Windows a bare "bash" can be the WSL launcher (System32 or WindowsApps),
// and running it starts WSL, so it is never used there.
var gitBashPaths = []string{
	`C:\Program Files\Git\bin\bash.exe`,
	`C:\Program Files\Git\usr\bin\bash.exe`,
}

var (
	winAbs     = regexp.MustCompile(`^[a-z]:\\`)
	winSystem  = regexp.MustCompile(`^[a-z]:\\windows\\(system32|sysnative|syswow64)\\`)
	errNoLookP = errors.New("LookPath must not be used on Windows")
)

func isSafeWindowsBash(p string) bool {
	n := strings.ToLower(strings.ReplaceAll(p, "/", `\`))
	if !winAbs.MatchString(n) || winSystem.MatchString(n) {
		return false
	}
	for _, seg := range strings.Split(n, `\`) {
		if seg == "windowsapps" {
			return false
		}
	}
	return true
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// resolveBash returns the bash to run, or "" and a skip reason.
func resolveBash(goos string, getenv func(string) string, exists func(string) bool, lookPath func(string) (string, error)) (string, string) {
	if goos != "windows" {
		p, err := lookPath("bash")
		if err != nil {
			return "", "bash not available"
		}
		return p, ""
	}
	if c := getenv("DOCK_TEST_BASH"); c != "" {
		if !isSafeWindowsBash(c) {
			return "", "DOCK_TEST_BASH=" + c + " is not an absolute path outside System32 and WindowsApps"
		}
		if !exists(c) {
			return "", "DOCK_TEST_BASH=" + c + " does not exist"
		}
		return c, ""
	}
	for _, p := range gitBashPaths {
		if exists(p) {
			return p, ""
		}
	}
	return "", "no Git Bash found; set DOCK_TEST_BASH (never the WSL bash)"
}

func TestResolveBash(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	has := func(paths ...string) func(string) bool {
		return func(p string) bool {
			for _, x := range paths {
				if x == p {
					return true
				}
			}
			return false
		}
	}
	look := func(p string) (string, error) { return "/usr/bin/" + p, nil }
	noLook := func(string) (string, error) { return "", errNoLookP }
	git, gitUsr := gitBashPaths[0], gitBashPaths[1]

	for _, goos := range []string{"linux", "darwin"} {
		if got, _ := resolveBash(goos, env(nil), has(), look); got != "/usr/bin/bash" {
			t.Errorf("%s: got %q", goos, got)
		}
	}
	if got, _ := resolveBash("windows", env(nil), has(git, gitUsr), noLook); got != git {
		t.Errorf("git bin: got %q", got)
	}
	if got, _ := resolveBash("windows", env(nil), has(gitUsr), noLook); got != gitUsr {
		t.Errorf("git usr: got %q", got)
	}
	if got, reason := resolveBash("windows", env(nil), has(), noLook); got != "" || !strings.Contains(reason, "DOCK_TEST_BASH") {
		t.Errorf("none found: %q %q", got, reason)
	}
	safe := `D:\tools\git\bin\bash.exe`
	if got, _ := resolveBash("windows", env(map[string]string{"DOCK_TEST_BASH": safe}), has(safe), noLook); got != safe {
		t.Errorf("safe override: got %q", got)
	}
	for _, bad := range []string{
		`C:\Windows\System32\bash.exe`,
		`c:/windows/system32/bash.exe`,
		`C:\WINDOWS\SYSTEM32\bash.exe`,
		`C:\Users\me\AppData\Local\Microsoft\WindowsApps\bash.exe`,
		`D:\x\windowsapps\bash.exe`,
		`bash`,
		`bash.exe`,
		`..\bash.exe`,
	} {
		got, reason := resolveBash("windows", env(map[string]string{"DOCK_TEST_BASH": bad}), has(bad, git), noLook)
		if got != "" || reason == "" {
			t.Errorf("%s: got %q %q", bad, got, reason)
		}
	}
	if got, _ := resolveBash("windows", env(map[string]string{"DOCK_TEST_BASH": `D:\nope\bash.exe`}), has(git), noLook); got != "" {
		t.Errorf("missing override: got %q", got)
	}
}
