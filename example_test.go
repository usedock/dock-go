package dock_test

import (
	"context"
	"errors"
	"fmt"

	dock "github.com/usedock/dock-go"
)

// Create a dock, run a command, read a file, and delete the dock.
// The client reads DOCK_API_KEY and DOCK_API_URL.
func Example() {
	ctx := context.Background()
	client := dock.NewClient()

	d, err := client.Docks.Create(ctx, dock.CreateParams{
		WorkspaceID: "your-workspace-id",
		Name:        "demo",
		CPUs:        2,
		MemoryMiB:   4096,
	})
	if err != nil {
		var apiErr *dock.Error
		if errors.As(err, &apiErr) {
			fmt.Println("API error:", apiErr.Code, apiErr.Status)
		}
		return
	}
	defer client.Docks.Delete(ctx, d.ID) //nolint:errcheck

	res, err := client.Docks.Exec(ctx, d.ID, dock.ExecParams{Command: "echo hello"})
	if err != nil {
		return
	}
	fmt.Print(res.Stdout)

	_ = client.Files.Write(ctx, d.ID, "/home/user/work/notes.txt", []byte("hi\n"))
	content, _ := client.Files.Read(ctx, d.ID, "/home/user/work/notes.txt")
	fmt.Print(string(content))
}
