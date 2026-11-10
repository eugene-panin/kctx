# kctx

Переключение Kubernetes-контекстов с группировкой по клиенту и TUI-поиском.

Имена вида `клиент/кластер` (`matchsystems.com/dc3`) группируются автоматически.
Источник правды — обычный kubeconfig (Lens/kubectl читают его как есть).

## Установка

```bash
go install github.com/eugene-panin/kctx/cmd/kctx@latest
# или из checkout:
go build -o kctx ./cmd/kctx && mv kctx "$(go env GOPATH)/bin/"
kctx completion zsh > "${fpath[1]}/_kctx"   # опционально
```

## Использование

```bash
kctx                 # TUI: фильтр, recent, избранное
kctx ls
kctx use NAME
kctx current
kctx clients

# добавить из файла или буфера
kctx add ./new.yml --client acme --name prod
pbpaste | kctx add - --client acme --name prod --use --check

kctx mv old-name acme/prod
kctx ns acme/prod apps
kctx export acme/prod > /tmp/acme-prod.yml
kctx check acme/prod
kctx fav acme/prod
```

### TUI

| клавиша | действие |
|---|---|
| enter | переключить контекст |
| p | вставить kubeconfig из буфера |
| n | задать namespace |
| f | в избранное / убрать |
| esc / q | выход |

Флаги: `--kubeconfig`, `--plain`, `--json`, `--no-input`.
Состояние (recent/favorites): `~/.local/state/kctx`.
