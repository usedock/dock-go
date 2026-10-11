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

// checkTruncated refuses output the API clipped (1 MiB per stream). A clipped
// base64 stream can still decode, so without this Read would return a prefix
// of the file as if it were all of it.
func checkTruncated(res *ExecResult, what string) error {
	if res.Truncated {
		return &Error{Code: "output_truncated", Status: 200, Message: what + ": output exceeded the exec output limit (1 MiB, about 768 KB of file content)"}
	}
	return nil
}

// writeCommand renders the docks.files.write template of sdk-scenarios/ops.yaml.
// The dirname runs with an "x" sentinel so a path ending in a newline keeps it
// ($(...) would strip it); "${d%??}" then drops the sentinel and its newline.
func writeCommand(path string, content []byte) string {
	q := shellWord(path)
	b64 := base64.StdEncoding.EncodeToString(content)
	return "d=$(dirname -- " + q + "; echo x) && mkdir -p -- \"${d%??}\" && printf '%s' '" + b64 + "' | base64 -d > " + q
}

// readCommand renders the docks.files.read template.
func readCommand(path string) string { return "base64 -w0 -- " + shellWord(path) }

// listCommand renders the docks.files.list template: a relative path gets ./ so
// find never reads it as an expression, -H follows a symlinked directory, and
// each entry name is printed base64-encoded on its own line.
func listCommand(path string) string {
	return "p=" + shellWord(path) + " && case $p in /*) ;; *) p=./$p ;; esac && test -d \"$p\" && " +
		"find -H \"$p\" -mindepth 1 -maxdepth 1 -exec sh -c 'for f; do printf \"%s\" \"${f##*/}\" | base64 -w0; echo; done' sh {} +"
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
	_, err := s.run(ctx, dockID, writeCommand(path, content))
	return err
}

// Read returns the content of the file at path. A file larger than about
// 768 KB is refused with code "output_truncated".
func (s *FilesService) Read(ctx context.Context, dockID, path string) ([]byte, error) {
	if err := checkPath(path); err != nil {
		return nil, err
	}
	res, err := s.run(ctx, dockID, readCommand(path))
	if err != nil {
		return nil, err
	}
	if err := checkTruncated(res, "read "+path); err != nil {
		return nil, err
	}
	b, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(res.Stdout))
	if derr != nil {
		return nil, &Error{Code: "bad_response", Status: 200, Message: "file content is not valid base64"}
	}
	return b, nil
}

// List returns the names in the directory at path, in no fixed order,
// including hidden entries. A path that is not a directory fails with code
// "file_command_failed".
func (s *FilesService) List(ctx context.Context, dockID, path string) ([]string, error) {
	if err := checkPath(path); err != nil {
		return nil, err
	}
	res, err := s.run(ctx, dockID, listCommand(path))
	if err != nil {
		return nil, err
	}
	if err := checkTruncated(res, "list "+path); err != nil {
		return nil, err
	}
	entries := []string{}
	for _, l := range strings.Split(res.Stdout, "\n") {
		if l == "" {
			continue
		}
		b, derr := base64.StdEncoding.DecodeString(l)
		if derr != nil {
			return nil, &Error{Code: "bad_response", Status: 200, Message: "directory entry is not valid base64"}
		}
		entries = append(entries, string(b))
	}
	return entries, nil
}
