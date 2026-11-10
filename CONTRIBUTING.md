# Contributing

Thanks for taking an interest in `kctx`.

## Development

```bash
git clone https://github.com/eugene-panin/kctx.git
cd kctx
go test ./...
go build -o kctx ./cmd/kctx
```

## Pull requests

1. Open an issue first for larger changes, so we can align on the approach.
2. Keep the diff focused — one concern per PR.
3. Run `gofmt`, `go test ./...`, and `go vet ./...` before pushing.
4. Update `README.md` / `SPEC.md` when you change user-facing behavior.
5. Do not commit binaries or kubeconfig files with credentials.

## Reporting bugs

Use a [bug report](https://github.com/eugene-panin/kctx/issues/new?template=bug_report.md)
and include the `kctx` version (`go version` / commit), OS, and the exact command.
