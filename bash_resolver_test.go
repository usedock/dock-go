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

var scoopSuffixes = []string{
	`\scoop\apps\git\current\usr\bin\bash.exe`,
	`\scoop\apps\git\current\bin\bash.exe`,
}

var (
	winAbs     = regexp.MustCompile(`^[a-z]:\\`)
	winSystem  = regexp.MustCompile(`\\windows\\(system32|sysnative|syswow64)\\`)
	errNoLookP = errors.New("LookPath must not be used on Windows")
)

func isSafeWindowsBash(p string) bool {
	n := strings.ToLower(strings.ReplaceAll(p, "/", `\`))
	if !winAbs.MatchString(n) || winSystem.MatchString(n) {
		return false
	}
	// Windows drops `.`/`..`, empty and trailing-dot segments and expands 8.3 names (WINDOW~1),
	// so only plain segments are accepted or the System32/WindowsApps check could be bypassed.
	// A `:` after the drive letter is an NTFS stream suffix (System32::$INDEX_ALLOCATION).
	for _, seg := range strings.Split(n, `\`)[1:] {
		if seg == "" || seg == "windowsapps" || strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") || strings.Contains(seg, "~") || strings.Contains(seg, ":") {
			return false
		}
	}
	return true
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// resolveBash returns the bash to run, the directories to put first on PATH
// (Git's usr\bin, so find and base64 are the GNU tools, not System32\find.exe),
// or "" and a skip reason.
func resolveBash(goos string, getenv func(string) string, exists func(string) bool, lookPath func(string) (string, error)) (bash, pathPrefix, reason string) {
	if goos != "windows" {
		p, err := lookPath("bash")
		if err != nil {
			return "", "", "bash not available"
		}
		return p, "", ""
	}
	if c := getenv("DOCK_TEST_BASH"); c != "" {
		if !isSafeWindowsBash(c) {
			return "", "", "DOCK_TEST_BASH=" + c + " is not an absolute path outside System32, SysWOW64, Sysnative and WindowsApps"
		}
		if !exists(c) {
			return "", "", "DOCK_TEST_BASH=" + c + " does not exist"
		}
		bash = c
	} else {
		candidates := append([]string{}, gitBashPaths...)
		if profile := getenv("USERPROFILE"); profile != "" {
			for _, s := range scoopSuffixes {
				candidates = append(candidates, profile+s)
			}
		}
		for _, p := range candidates {
			if exists(p) {
				bash = p
				break
			}
		}
		if bash == "" {
			return "", "", "no Git Bash found; set DOCK_TEST_BASH (never the WSL bash)"
		}
	}
	bash = strings.ReplaceAll(bash, "/", `\`)
	dir := bash[:strings.LastIndex(bash, `\`)]
	if i := strings.LastIndex(dir, `\`); i >= 0 {
		return bash, dir[:i] + `\usr\bin` + ";" + dir, ""
	}
	return bash, dir, ""
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
	resolve := func(goos string, getenv func(string) string, exists func(string) bool, lp func(string) (string, error)) (string, string) {
		b, _, r := resolveBash(goos, getenv, exists, lp)
		return b, r
	}

	for _, goos := range []string{"linux", "darwin"} {
		if got, _ := resolve(goos, env(nil), has(), look); got != "/usr/bin/bash" {
			t.Errorf("%s: got %q", goos, got)
		}
	}
	if got, _ := resolve("windows", env(nil), has(git, gitUsr), noLook); got != git {
		t.Errorf("git bin: got %q", got)
	}
	if got, _ := resolve("windows", env(nil), has(gitUsr), noLook); got != gitUsr {
		t.Errorf("git usr: got %q", got)
	}
	if got, reason := resolve("windows", env(nil), has(), noLook); got != "" || !strings.Contains(reason, "DOCK_TEST_BASH") {
		t.Errorf("none found: %q %q", got, reason)
	}
	safe := `D:\tools\git\bin\bash.exe`
	if got, _ := resolve("windows", env(map[string]string{"DOCK_TEST_BASH": safe}), has(safe), noLook); got != safe {
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
		`C:\Windows\.\System32\bash.exe`,
		`C:\Windows\System32.\bash.exe`,
		`C:\Windows\\System32\bash.exe`,
		`C:\Windows\System32\..\System32\bash.exe`,
		`C:\Users\me\AppData\Local\Microsoft\WINDOW~1\bash.exe`,
		`C:\Users\me\AppData\Local\Microsoft\WindowsApps \bash.exe`,
		`C:\Windows\System32::$INDEX_ALLOCATION\bash.exe`,
		`C:\Windows\System32:$I30:$INDEX_ALLOCATION\bash.exe`,
		`C:\Windows::$INDEX_ALLOCATION\System32\bash.exe`,
		`C:\Users\me\AppData\Local\Microsoft\WindowsApps::$INDEX_ALLOCATION\bash.exe`,
		`\\?\C:\Windows\System32\bash.exe`,
		`\\.\C:\Windows\System32\bash.exe`,
		`//?/C:/Windows/System32/bash.exe`,
		`\\localhost\C$\Windows\System32\bash.exe`,
	} {
		got, reason := resolve("windows", env(map[string]string{"DOCK_TEST_BASH": bad}), has(bad, git), noLook)
		if got != "" || reason == "" {
			t.Errorf("%s: got %q %q", bad, got, reason)
		}
	}
	if got, _ := resolve("windows", env(map[string]string{"DOCK_TEST_BASH": `D:\nope\bash.exe`}), has(git), noLook); got != "" {
		t.Errorf("missing override: got %q", got)
	}
}

func TestResolveBashScoopAndPath(t *testing.T) {
	bs := string(rune(92))
	profile := "C:" + bs + "Users" + bs + "me"
	cur := profile + bs + "scoop" + bs + "apps" + bs + "git" + bs + "current"
	want := cur + bs + "bin" + bs + "bash.exe"
	env := func(k string) string {
		if k == "USERPROFILE" {
			return profile
		}
		return ""
	}
	bash, prefix, _ := resolveBash("windows", env, func(p string) bool { return p == want }, nil)
	if bash != want || prefix != cur+bs+"usr"+bs+"bin;"+cur+bs+"bin" {
		t.Errorf("got %q %q", bash, prefix)
	}
	// A forward-slash override must not panic and is normalised.
	over := func(k string) string {
		if k == "DOCK_TEST_BASH" {
			return "D:/g/bin/bash.exe"
		}
		return ""
	}
	bash, _, reason := resolveBash("windows", over, func(string) bool { return true }, nil)
	if bash != "D:"+bs+"g"+bs+"bin"+bs+"bash.exe" {
		t.Errorf("got %q %q", bash, reason)
	}
	for _, bad := range []string{"C:/Windows/SysWOW64/bash.exe", "C:/Windows/Sysnative/bash.exe"} {
		bad := bad
		get := func(k string) string {
			if k == "DOCK_TEST_BASH" {
				return bad
			}
			return ""
		}
		if got, _, _ := resolveBash("windows", get, func(string) bool { return true }, nil); got != "" {
			t.Errorf("%s accepted", bad)
		}
	}
}
