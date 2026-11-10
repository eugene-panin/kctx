# kctx spec

## Purpose
Оператор с десятками кластеров у многих клиентов быстро находит нужный контекст и делает его текущим. Human-first; скрипты — вторично.

## Name
`kctx`

## Commands
```text
kctx                 TUI-пикер
kctx ls              список по клиентам
kctx use <name>      переключить
kctx current         текущий контекст
kctx clients         клиенты + число кластеров
```

## Model
- Источник: `--kubeconfig` / `KUBECONFIG` / `~/.kube/config`
- Client = часть имени до первого `/`; без `/` — имя целиком
- v0 меняет только `current-context`

## Output
`--plain`, `--json`, `NO_COLOR` через lipgloss/терминал.

## Danger
Низкая. Confirm не нужен.

## Deviations
| rule | instead | why |
|---|---|---|
| O12 pager | нет | список обычно на один экран; TUI сам скроллит |
| fuzzy lib | substring | достаточно для имён клиент/кластер; меньше зависимости |
