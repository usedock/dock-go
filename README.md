# Dock Go SDK

The Go client for [Dock](https://github.com/usedock/dock): isolated Linux machines for agents, on your laptop, your servers, or our cloud.

> **Status:** not published yet. The module path is final, but there is no tagged release; expect changes until `v0.1.0`.

## Install

```sh
go get github.com/usedock/dock-go
```

Requires Go 1.22 or newer. The runtime uses only the standard library.

## Example

Create a dock, run a command, write and read a file, and delete it.

```go
package main

import (
	"context"
	"fmt"
	"log"

	dock "github.com/usedock/dock-go"
)

func main() {
	ctx := context.Background()
	client := dock.NewClient() // reads DOCK_API_KEY and DOCK_API_URL

	d, err := client.Docks.Create(ctx, dock.CreateParams{
		WorkspaceID: "your-workspace-id",
		Name:        "demo",
		CPUs:        2,
		MemoryMiB:   4096,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Docks.Delete(ctx, d.ID)

	res, err := client.Docks.Exec(ctx, d.ID, dock.ExecParams{Command: "echo hello"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(res.Stdout) // hello

	_ = client.Files.Write(ctx, d.ID, "/home/user/work/notes.txt", []byte("hi\n"))
	b, _ := client.Files.Read(ctx, d.ID, "/home/user/work/notes.txt")
	fmt.Print(string(b))
}
```

## API

Every call takes a `context.Context`; create, stop, resume and branch return when the work is done, so bound them with the context.

| Call | Route |
|---|---|
| `client.Docks.Create / List / Get / Update` | `POST/GET /v1/docks`, `GET/PATCH /v1/docks/{id}` |
| `client.Docks.Exec` | `POST /v1/docks/{id}/exec` |
| `client.Docks.Stop / Resume / Branch` | `POST /v1/docks/{id}/stop`, `/resume`, `/branch` |
| `client.Docks.Delete` | `DELETE /v1/docks/{id}` (sends the `x-confirm-delete` header; returns an operation) |
| `client.Operations.Get` | `GET /v1/operations/{id}` |
| `client.Connectors.List / Connect / Disconnect` | `GET /v1/workspaces/{id}/connectors`, `PUT/DELETE /v1/workspaces/{id}/connectors/{kind}` |
| `client.Environments.List / Get / Update` | `GET /v1/environments`, `GET/PATCH /v1/environments/{ref}` |
| `client.Ports.List / Expose / Remove` | `GET/POST /v1/docks/{id}/ports`, `DELETE /v1/docks/{id}/ports/{port}` |
| `client.Images.List` | `GET /v1/images` |
| `client.Files.Write / Read / List` | built on exec, see below |

Auto-stop: `TTLSeconds` sets it (counted from ready) on `Update` and `Resume`, `ClearTTL: true` sends `ttlSeconds: null` to turn it off, and leaving both unset keeps the current value. Setting both is refused client-side with code `invalid_argument`.

The API has no file routes. `Files` runs one `Docks.Exec` per call with the paths shell-quoted and the content sent as base64, so the dock must be running and one write is limited to about 48 KB (larger content fails with code `file_too_large` before anything is sent). A read is limited to about 768 KB of file content (the API clips exec output at 1 MiB), and clipped output from `Read` or `List` fails with code `output_truncated` instead of returning partial data. A non-zero exit from the command is an error with code `file_command_failed` (this includes `List` on a path that is not a directory). These codes match the "File helper errors" table in the shared scenarios. `List` returns names in no fixed order, including hidden entries; names are base64-encoded on the wire, so names with newlines or other odd bytes survive.

Response structs ignore fields they do not know, and only `id`, `name` and `state` are relied upon.

## Ports

Expose a TCP port inside a dock at a URL. A new port is private: the URL needs a dock-bound API key or an organization member's sign-in. Set `Visibility` to open it to anyone with the URL.

```go
p, _ := client.Ports.Expose(ctx, d.ID, 3000, dock.ExposePortParams{}) // p.Visibility == "private"

// Exposing again with a visibility changes it; leaving it empty keeps the current one.
p, _ = client.Ports.Expose(ctx, d.ID, 3000, dock.ExposePortParams{Visibility: dock.PortVisibilityPublic})

ps, _ := client.Ports.List(ctx, d.ID) // port, visibility, url, createdAt
_ = client.Ports.Remove(ctx, d.ID, 3000)
```

`URL` is nil while hosted ports are off on the deployment.

## Environments

`client.Environments.List / Get / Update` read and change environments. `ProviderKeys` is `dock.ProviderKeysGateway` (default: provider keys stay in the control plane and docks get a placeholder key plus gateway base URLs) or `dock.ProviderKeysPlain` (the real key is written into the dock, so anyone inside it can read it; admins only). Connectors also report `CredentialType`.

## Connectors and images

Connect a tool to a workspace with a token (organization admins only), attach it to a dock by kind, and list the Dockyard images:

```go
imgs, _ := client.Images.List(ctx) // name, description, arch, sizeMiB, contents, available

_, err := client.Connectors.Connect(ctx, "your-workspace-id", "slack", "xoxb-...")
cs, _ := client.Connectors.List(ctx, "your-workspace-id") // kind, type, status, ...

d, err := client.Docks.Create(ctx, dock.CreateParams{
	WorkspaceID: "your-workspace-id",
	Image:       "node-22",
	Connectors:  []string{"slack"}, // d.Connectors echoes the attached kinds
})
var apiErr *dock.Error
if errors.As(err, &apiErr) && apiErr.Code == dock.CodeConnectorNotConnected {
	// connect it first
}

_, _ = client.Connectors.Disconnect(ctx, "your-workspace-id", "slack")
```

Error codes: `dock.CodeConnectorNotConnected` (409), `dock.CodeConnectorAdminOnly` (403), `dock.CodeConnectorNotFound` (404, from `Disconnect`), `dock.CodeConnectorTokenInvalid` (400), `dock.CodeConnectorUnreachable` (502), `dock.CodeImageUnknown` (400), `dock.CodeImageUnavailable` (400), `dock.CodePortNotFound` (404), `dock.CodePortLimitReached` (409), `dock.CodePortsUnavailable` (503).

Port 2222 is reserved for the dock agent and is rejected with `validation_error`. A dock can expose at most 10 ports.

## Configuration

| Setting | Option | Environment | Default |
|---|---|---|---|
| API key (`dock_...`) | `dock.WithAPIKey` | `DOCK_API_KEY` | none; the first call fails with `missing_api_key` |
| Control plane origin, without `/v1` | `dock.WithBaseURL` | `DOCK_API_URL` | `https://api.usedock.io` |
| HTTP client | `dock.WithHTTPClient` | | `http.Client` with no timeout |

Options override the environment.

## Errors

API failures are `*dock.Error` with `Code` (stable, for example `dock_not_found`), `Status` (HTTP status) and `Message`:

```go
_, err := client.Docks.Get(ctx, id)
var apiErr *dock.Error
if errors.As(err, &apiErr) && apiErr.Code == "dock_not_found" {
	// ...
}
```

Network failures and cancelled contexts are returned as ordinary errors.

## Tests and shared scenarios

```sh
go test ./...
```

The SDKs share one scenario suite, `sdk-scenarios/` in [usedock/dock](https://github.com/usedock/dock). The scenario test loads it from `DOCK_SCENARIOS_DIR` (default `../dock/sdk-scenarios`) and skips when it is absent. The commit CI uses is pinned in `SCENARIOS_REF`.

- Offline: every scenario replays its recorded fixture against a local fake server.
- Live: with `DOCK_API_URL`, `DOCK_API_KEY` and `DOCK_WORKSPACE_ID` set, the same scenarios run against a real control plane. They create and delete docks named `sdk-...`, so use a throwaway workspace.

```sh
git clone --depth 1 https://github.com/usedock/dock ../dock
DOCK_SCENARIOS_DIR=../dock/sdk-scenarios go test -run TestScenarios -v ./...
```

## Design

- Written by hand, not generated, so names read the way you would say them: `Docks.Create`, `Docks.Exec`, `Docks.Branch`.
- Only public routes. Fleet, admin and internal routes never appear.
- The same nouns and verbs as the [TypeScript](https://github.com/usedock/dock-js) and [Python](https://github.com/usedock/dock-python) SDKs, checked by the shared scenario suite.
- Semver; a major version matches an API version.

## Contributing

Contributions are welcome. Every contributor signs the CLA; the CLA bot asks on your first pull request in each repository. API changes start in [usedock/dock](https://github.com/usedock/dock) and link their SDK pull requests.

## Security

Report vulnerabilities to [security@usedock.io](mailto:security@usedock.io), not in public issues.

## License

[Apache-2.0](LICENSE).

Copyright (C) 2026 Paranjay.
