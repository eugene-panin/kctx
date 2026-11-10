# kctx

Переключение Kubernetes-контекстов с группировкой по клиенту и TUI-поиском.

Имена вида `клиент/кластер` (`matchsystems.com/dc3`) группируются автоматически.

## Установка

```bash
go install kctx/cmd/kctx@latest   # из модуля
# или
go build -o kctx ./cmd/kctx && mv kctx "$(go env GOPATH)/bin/"
```

## Использование

```bash
kctx                 # TUI: фильтр → Enter
kctx ls              # список по клиентам (* = текущий)
kctx use NAME        # переключить без TUI
kctx current
kctx clients
```

Флаги: `--kubeconfig`, `--plain`, `--json`, `--no-input`.
