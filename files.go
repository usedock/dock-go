package dock

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
)

// maxCommandBytes is the exec command limit (64 KB). It caps a single Write at
// about 48 KB of content, because content travels as base64 inside the command.
const maxCommandBytes = 64 * 1024

// FilesService reads and writes files in a dock. The API has no file routes:
// each call is one Docks.Exec running a shell command, so the dock must be
// running. Get it from Client.Files.
type FilesService struct{ c *Client }

// shellWord quotes s as one POSIX shell word: single quotes, with each '
// replaced by the four characters quote, backslash, quote, quote.
func shellWord(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (s *FilesService) run(ctx context.Context, dockID, command string) (*ExecResult, error) {
	if len(command) > maxCommandBytes {
		return nil, &Error{Code: "file_too_large", Message: fmt.Sprintf("command is %d bytes; the limit is %d (about 48 KB of content per write)", len(command), maxCommandBytes)}
	}
	res, err := s.c.Docks.Exec(ctx, dockID, ExecParams{Command: command})
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, &Error{Code: "file_command_failed", Status: 200, Message: fmt.Sprintf("exit code %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))}
	}
	return res, nil
}

func checkPath(path string) error {
	if path == "" {
		return &Error{Code: "invalid_argument", Message: "path is empty"}
	}
	return nil
}

// Write writes content to path, creating parent directories. Content larger
// than about 48 KB is refused with code "file_too_large". A non-zero exit
// from the underlying command is an *Error with code "file_command_failed".
func (s *FilesService) Write(ctx context.Context, dockID, path string, content []byte) error {
	if err := checkPath(path); err != nil {
		return err
	}
	q := shellWord(path)
	b64 := base64.StdEncoding.EncodeToString(content)
	cmd := "mkdir -p -- \"$(dirname -- " + q + ")\" && printf '%s' '" + b64 + "' | base64 -d > " + q
	_, err := s.run(ctx, dockID, cmd)
	return err
}

// Read returns the content of the file at path.
func (s *FilesService) Read(ctx context.Context, dockID, path string) ([]byte, error) {
	if err := checkPath(path); err != nil {
		return nil, err
	}
	res, err := s.run(ctx, dockID, "base64 -w0 -- "+shellWord(path))
	if err != nil {
		return nil, err
	}
	b, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(res.Stdout))
	if derr != nil {
		return nil, &Error{Code: "bad_response", Status: 200, Message: "file content is not valid base64"}
	}
	return b, nil
}

// List returns the names in the directory at path, in ls order, including
// hidden entries.
func (s *FilesService) List(ctx context.Context, dockID, path string) ([]string, error) {
	if err := checkPath(path); err != nil {
		return nil, err
	}
	res, err := s.run(ctx, dockID, "ls -1A -- "+shellWord(path))
	if err != nil {
		return nil, err
	}
	entries := []string{}
	for _, l := range strings.Split(res.Stdout, "\n") {
		if l != "" {
			entries = append(entries, l)
		}
	}
	return entries, nil
}
