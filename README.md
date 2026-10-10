# Dock Go SDK

The Go client for [Dock](https://github.com/usedock/dock): isolated Linux machines for agents, on your laptop, your servers, or our cloud.

> **Status:** not released yet. This SDK is being written by hand.

## Install

```sh
go get github.com/usedock/dock-go
```

## Example

```go
client := dock.NewClient() // reads DOCK_API_KEY and the current context

d, err := client.Docks.Create(ctx, dock.CreateOptions{CPUs: 2, Memory: "4gb"})
if err != nil {
    return err
}
run, err := d.Exec(ctx, "bun test")
fmt.Println(run.Stdout)
_ = d.Stop(ctx)
```

The context is a self-hosted control plane, or Dock Cloud (coming soon).

## Design

- Written by hand, not generated, so names read the way you would say them: `Docks.Create`, `d.Exec`, `d.Branch`.
- Only public routes. Fleet, admin and internal routes never appear.
- The same nouns and verbs as the [TypeScript](https://github.com/usedock/dock-js) and [Python](https://github.com/usedock/dock-python) SDKs, and one shared scenario suite (create, exec, stop, resume, branch, delete) run against a real control plane.
- Semver; a major version matches an API version.

## Contributing

Contributions are welcome. Every contributor signs the CLA once; the CLA bot asks on your first pull request. API changes start in [usedock/dock](https://github.com/usedock/dock) and link their SDK pull requests.

## Security

Report vulnerabilities to [security@usedock.io](mailto:security@usedock.io), not in public issues.

## License

[Apache-2.0](LICENSE).
