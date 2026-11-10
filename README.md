# kctx

Switch Kubernetes contexts with client grouping and a searchable TUI.

Names like `client/cluster` (`matchsystems.com/dc3`) are grouped automatically.
The source of truth is a normal kubeconfig — Lens and kubectl read it as usual.

## Install

```bash
brew tap eugene-panin/tap
brew install --HEAD eugene-panin/tap/kctx
```

Or with Go:

```bash
go install github.com/eugene-panin/kctx/cmd/kctx@latest
# from a checkout:
go build -o kctx ./cmd/kctx && mv kctx "$(go env GOPATH)/bin/"
kctx completion zsh > "${fpath[1]}/_kctx"   # optional
```

## Usage

```bash
kctx                 # TUI: filter, recent, favorites
kctx ls
kctx use NAME
kctx current
kctx clients

# add from a file or the clipboard
kctx add ./new.yml --client acme --name prod
pbpaste | kctx add - --client acme --name prod --use --check

kctx mv old-name acme/prod
kctx ns acme/prod apps
kctx export acme/prod > /tmp/acme-prod.yml
kctx check acme/prod
kctx fav acme/prod
```

### TUI

| key | action |
|---|---|
| enter | switch context |
| p | paste kubeconfig from clipboard |
| n | set namespace |
| f | toggle favorite |
| esc / q | quit |

Flags: `--kubeconfig`, `--plain`, `--json`, `--no-input`.  
State (recent/favorites): `~/.local/state/kctx`.
