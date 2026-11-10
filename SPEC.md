# kctx spec

## Purpose
Оператор с десятками кластеров у многих клиентов быстро находит нужный контекст и делает его текущим. Human-first; скрипты — вторично.

## Name
`kctx`

## Commands
```text
kctx                              TUI-пикер (recent / favorites / paste / ns)
kctx add <file|-> --client C      мерж → C/short (stdin: -)
kctx ls                           список по клиентам
kctx use <name>                   переключить (+ recent)
kctx current                      текущий контекст
kctx clients                      клиенты + число кластеров
kctx mv <from> <to>               переименовать в client/cluster
kctx ns [ctx] [namespace]         показать / задать default ns
kctx export <ctx>...              kubeconfig на stdout
kctx check [ctx]...               /version probe
kctx fav [ctx]                    список / toggle избранного
kctx completion bash|zsh|fish     shell completions
```

## add
- Synopsis: `kctx add <kubeconfig|-> --client <name> [--name <short>] [--use] [--force] [--dry-run] [--check]`
- `-` читает stdin (`pbpaste | kctx add - --client acme`)
- Short-имя санитизируется (EKS ARN → последний сегмент, `@` → `-`)
- Нет contexts, но 1 cluster + 1 user → контекст выводится
- stdout: имена; stderr: `added` / `dry-run` / `overwrote` / `switched` / check
- Danger: средняя; `--dry-run`, `--force`

## mv / ns / export / check / fav
- `mv` требует `to` вида `client/cluster`; тащит cluster/user keys; обновляет current
- `ns` без записи только читает; `-` сбрасывает namespace
- `export` — self-contained kubeconfig, credentials на stdout (не логировать)
- `check` — ClientSet Discovery ServerVersion, `--timeout` (default 10s), exit 1 если fail
- `fav` / recent в `$XDG_STATE_HOME/kctx` (иначе `~/.local/state/kctx`)

## TUI
- секции: ★ избранное, недавние, все (по клиентам)
- `enter` use · `p` paste из буфера (client → optional name) · `n` namespace · `f` favorite · `esc`/`q`

## Model
- Источник правды: `--kubeconfig` / `KUBECONFIG` / `~/.kube/config` (не отдельный store)
- Client = часть имени до первого `/`

## Output
`--plain`, `--json`, completions через cobra.

## Danger
`use`/`ns`/`fav` — низкая. `add`/`mv` — средняя. `export` печатает секреты на stdout.

## Deviations
| rule | instead | why |
|---|---|---|
| O12 pager | нет | список/TUI сами укладываются |
| fuzzy lib | substring | достаточно для client/cluster |
