package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/eugene-panin/kctx/internal/kube"
	"github.com/eugene-panin/kctx/internal/recent"
	"github.com/eugene-panin/kctx/internal/tui"
)

const (
	exitOK        = 0
	exitFailure   = 1
	exitUsage     = 2
	exitInterrupt = 130
)

type userError struct {
	Err  error
	Hint string
	Code int
}

func (e *userError) Error() string { return e.Err.Error() }
func (e *userError) Unwrap() error { return e.Err }

func usagef(hint, format string, args ...any) error {
	return &userError{Err: fmt.Errorf(format, args...), Hint: hint, Code: exitUsage}
}

func failf(hint, format string, args ...any) error {
	return &userError{Err: fmt.Errorf(format, args...), Hint: hint, Code: exitFailure}
}

// Run is the testable entrypoint. Returns a process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	a := &application{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		getenv: getenv,
	}
	root := a.rootCmd()
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	return report(root.Execute(), stderr)
}

func report(err error, stderr io.Writer) int {
	var ue *userError
	switch {
	case err == nil:
		return exitOK
	case errors.As(err, &ue):
		fmt.Fprintf(stderr, "kctx: %v\n", ue.Err)
		if ue.Hint != "" {
			fmt.Fprintln(stderr, ue.Hint)
		}
		return ue.Code
	default:
		fmt.Fprintf(stderr, "kctx: %v\n", err)
		return exitFailure
	}
}

type application struct {
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
	getenv     func(string) string
	kubeconfig string
	plain      bool
	jsonOut    bool
	noInput    bool
}

func (a *application) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "kctx",
		Short:         "Переключение kube-контекстов по клиентам",
		Long:          "kctx уменьшает путаницу между десятками кластеров: группирует контексты по клиенту (часть имени до '/') и даёт TUI-поиск.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runPicker()
		},
	}
	root.PersistentFlags().StringVar(&a.kubeconfig, "kubeconfig", "", "путь к kubeconfig (иначе KUBECONFIG или ~/.kube/config)")
	root.PersistentFlags().BoolVar(&a.noInput, "no-input", false, "запретить интерактивный ввод")
	root.PersistentFlags().BoolVar(&a.plain, "plain", false, "машиночитаемый вывод без оформления")
	root.PersistentFlags().BoolVar(&a.jsonOut, "json", false, "JSON на stdout")

	root.AddCommand(
		a.addCmd(),
		a.lsCmd(),
		a.useCmd(),
		a.currentCmd(),
		a.clientsCmd(),
		a.mvCmd(),
		a.nsCmd(),
		a.exportCmd(),
		a.checkCmd(),
		a.favCmd(),
	)
	return root
}

func (a *application) resolvePath() string {
	if a.kubeconfig != "" {
		return a.kubeconfig
	}
	return a.getenv("KUBECONFIG")
}

func (a *application) completeContexts(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	entries, err := kube.Load(a.resolvePath())
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name, toComplete) {
			out = append(out, e.Name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func (a *application) runPicker() error {
	if a.noInput || !isatty.IsTerminal(os.Stdin.Fd()) || !isatty.IsTerminal(os.Stdout.Fd()) {
		return usagef("запустите: kctx ls   или   kctx use <context>", "TUI нужен интерактивный терминал")
	}
	entries, err := kube.Load(a.resolvePath())
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return failf("добавьте контексты: kctx add <file> --client …", "контекстов нет")
	}
	rec, _ := recent.List()
	fav, _ := recent.Favorites()
	res, err := tui.Run(tui.Options{
		Entries:   entries,
		Recent:    rec,
		Favorites: fav,
		KubePath:  a.resolvePath(),
		Stdin:     a.stdin,
		Stderr:    a.stderr,
	})
	if err != nil {
		return err
	}
	if res.Canceled {
		return nil
	}
	switch res.Action {
	case tui.ActionPaste:
		return a.addFromBytes(res.PasteData, res.PasteClient, res.PasteName, true)
	case tui.ActionNamespace:
		if err := kube.SetNamespace(a.resolvePath(), res.Selected, res.Namespace); err != nil {
			return failf("проверьте имя ns", "%v", err)
		}
		fmt.Fprintf(a.stderr, "namespace %s → %s\n", res.Selected, emptyDash(res.Namespace))
		return nil
	case tui.ActionFavorite:
		on, err := recent.ToggleFavorite(res.Selected)
		if err != nil {
			return err
		}
		if on {
			fmt.Fprintf(a.stderr, "favorited %s\n", res.Selected)
		} else {
			fmt.Fprintf(a.stderr, "unfavorited %s\n", res.Selected)
		}
		return nil
	default:
		if err := kube.Use(a.resolvePath(), res.Selected); err != nil {
			return err
		}
		_ = recent.Touch(res.Selected)
		fmt.Fprintln(a.stdout, res.Selected)
		fmt.Fprintf(a.stderr, "switched to %s\n", res.Selected)
		return nil
	}
}

func emptyDash(s string) string {
	if s == "" {
		return "(default)"
	}
	return s
}

func (a *application) addFromBytes(data []byte, client, name string, use bool) error {
	if client == "" {
		return usagef("пример: pbpaste | kctx add - --client acme --name prod", "нужен --client")
	}
	res, err := kube.Add(kube.AddOptions{
		DestPath:   a.resolvePath(),
		SourcePath: "-",
		Source:     strings.NewReader(string(data)),
		Client:     client,
		Name:       name,
		Use:        use,
	})
	if err != nil {
		return failf("проверьте kubeconfig в буфере и --client", "%v", err)
	}
	for _, n := range res.Added {
		fmt.Fprintln(a.stdout, n)
	}
	fmt.Fprintf(a.stderr, "added %d context(s)\n", len(res.Added))
	if use && len(res.Added) == 1 {
		_ = recent.Touch(res.Added[0])
		fmt.Fprintf(a.stderr, "switched to %s\n", res.Added[0])
	}
	return nil
}

func (a *application) addCmd() *cobra.Command {
	var (
		client string
		name   string
		use    bool
		force  bool
		dryRun bool
		check  bool
	)
	cmd := &cobra.Command{
		Use:   "add <kubeconfig|->",
		Short: "Добавить контексты из файла или stdin",
		Long: `Читает kubeconfig и мержит контексты в основной конфиг как client/short.

Имена коротких кластеров санитизируются (EKS ARN → последний сегмент).
«-» читает stdin (удобно: pbpaste | kctx add - --client acme).

Примеры:
  kctx add ./new.yml --client acme --name prod
  pbpaste | kctx add - --client acme --name prod --use
  kctx add ./bundle.yml --client acme --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := kube.AddOptions{
				DestPath:   a.resolvePath(),
				SourcePath: args[0],
				Client:     client,
				Name:       name,
				Use:        use,
				Force:      force,
				DryRun:     dryRun,
			}
			if args[0] == "-" {
				opts.Source = a.stdin
			}
			res, err := kube.Add(opts)
			if err != nil {
				return failf("проверьте файл и флаги: kctx add -h", "%v", err)
			}
			for _, n := range res.Added {
				fmt.Fprintln(a.stdout, n)
			}
			if dryRun {
				fmt.Fprintf(a.stderr, "dry-run: would add %d context(s)", len(res.Added))
				if len(res.Overwrites) > 0 {
					fmt.Fprintf(a.stderr, ", overwrite %d", len(res.Overwrites))
				}
				fmt.Fprintln(a.stderr)
				return nil
			}
			if len(res.Overwrites) > 0 {
				fmt.Fprintf(a.stderr, "overwrote: %s\n", strings.Join(res.Overwrites, ", "))
			}
			fmt.Fprintf(a.stderr, "added %d context(s)\n", len(res.Added))
			if use && len(res.Added) == 1 {
				_ = recent.Touch(res.Added[0])
				fmt.Fprintf(a.stderr, "switched to %s\n", res.Added[0])
			}
			if check {
				var failed bool
				for _, n := range res.Added {
					cr := kube.Check(a.resolvePath(), n, 10*time.Second)
					if a.jsonOut {
						_ = json.NewEncoder(a.stdout).Encode(cr)
						continue
					}
					if cr.OK {
						fmt.Fprintf(a.stderr, "check %s: ok %s (%dms)\n", cr.Name, cr.Version, cr.Latency)
					} else {
						failed = true
						fmt.Fprintf(a.stderr, "check %s: %s (%dms)\n", cr.Name, cr.Error, cr.Latency)
					}
				}
				if failed {
					return failf("кластер добавлен, но check не прошёл", "один или несколько check failed")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&client, "client", "", "имя клиента (обязательно)")
	cmd.Flags().StringVar(&name, "name", "", "короткое имя кластера (только если в файле один контекст)")
	cmd.Flags().BoolVar(&use, "use", false, "сразу сделать добавленный контекст текущим")
	cmd.Flags().BoolVar(&force, "force", false, "перезаписать одноимённый контекст")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "показать имена, не писать kubeconfig")
	cmd.Flags().BoolVar(&check, "check", false, "проверить API после добавления")
	_ = cmd.MarkFlagRequired("client")
	return cmd
}

func (a *application) lsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "Список контекстов, сгруппированных по клиенту",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := kube.Load(a.resolvePath())
			if err != nil {
				return err
			}
			if a.jsonOut {
				return json.NewEncoder(a.stdout).Encode(entries)
			}
			prev := ""
			for _, e := range entries {
				if !a.plain && e.Client != prev {
					if prev != "" {
						fmt.Fprintln(a.stdout)
					}
					fmt.Fprintf(a.stdout, "%s\n", e.Client)
					prev = e.Client
				}
				mark := " "
				if e.Current {
					mark = "*"
				}
				if a.plain {
					fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", mark, e.Client, e.Name)
					continue
				}
				ns := e.Namespace
				if ns == "" {
					ns = "-"
				}
				fmt.Fprintf(a.stdout, "  %s %s  ns/%s\n", mark, e.Short, ns)
			}
			return nil
		},
	}
}

func (a *application) useCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "use <context>",
		Short:             "Сделать контекст текущим",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: a.completeContexts,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := kube.Use(a.resolvePath(), name); err != nil {
				return failf("проверьте имя: kctx ls", "%v", err)
			}
			_ = recent.Touch(name)
			fmt.Fprintln(a.stdout, name)
			fmt.Fprintf(a.stderr, "switched to %s\n", name)
			return nil
		},
	}
	return cmd
}

func (a *application) currentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Показать текущий контекст",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := kube.Current(a.resolvePath())
			if err != nil {
				return err
			}
			if name == "" {
				return failf("выберите контекст: kctx", "current-context не задан")
			}
			client, short := kube.SplitClient(name)
			if a.jsonOut {
				return json.NewEncoder(a.stdout).Encode(map[string]string{
					"name": name, "client": client, "short": short,
				})
			}
			if a.plain {
				fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", client, short, name)
				return nil
			}
			fmt.Fprintf(a.stdout, "%s (%s)\n", name, client)
			return nil
		},
	}
}

func (a *application) clientsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clients",
		Short: "Список клиентов и число кластеров",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := kube.Load(a.resolvePath())
			if err != nil {
				return err
			}
			clients := kube.Clients(entries)
			if a.jsonOut {
				return json.NewEncoder(a.stdout).Encode(clients)
			}
			for _, c := range clients {
				if a.plain {
					fmt.Fprintf(a.stdout, "%s\t%d\n", c.Name, c.Count)
					continue
				}
				fmt.Fprintf(a.stdout, "%s  (%d)\n", c.Name, c.Count)
			}
			return nil
		},
	}
}

func (a *application) mvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mv <from> <to>",
		Short: "Переименовать контекст в client/cluster",
		Long:  "Пример: kctx mv kubernetes-admin-kubernetes acme/prod",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return a.completeContexts(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := kube.Move(a.resolvePath(), args[0], args[1]); err != nil {
				return failf("to должен быть client/cluster", "%v", err)
			}
			fmt.Fprintf(a.stdout, "%s\n", args[1])
			fmt.Fprintf(a.stderr, "moved %s → %s\n", args[0], args[1])
			_ = recent.Touch(args[1])
			return nil
		},
	}
	return cmd
}

func (a *application) nsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ns [context] [namespace]",
		Short: "Показать или задать default namespace",
		Long: `Без аргументов — namespace текущего контекста.
Один аргумент-контекст — его namespace.
Два аргумента — установить namespace ("" или "-" сбрасывает).`,
		Args: cobra.MaximumNArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return a.completeContexts(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch len(args) {
			case 0, 1:
				name := ""
				if len(args) == 1 {
					name = args[0]
				}
				ns, err := kube.Namespace(a.resolvePath(), name)
				if err != nil {
					return failf("kctx ls", "%v", err)
				}
				if ns == "" {
					ns = "-"
				}
				fmt.Fprintln(a.stdout, ns)
				return nil
			default:
				ns := args[1]
				if ns == "-" {
					ns = ""
				}
				if err := kube.SetNamespace(a.resolvePath(), args[0], ns); err != nil {
					return failf("namespace — DNS label", "%v", err)
				}
				fmt.Fprintf(a.stderr, "namespace %s → %s\n", args[0], emptyDash(ns))
				return nil
			}
		},
	}
	return cmd
}

func (a *application) exportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "export <context>...",
		Short:             "Выгрузить контексты как отдельный kubeconfig",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: a.completeContexts,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := kube.Export(a.resolvePath(), args)
			if err != nil {
				return failf("проверьте имена: kctx ls", "%v", err)
			}
			b, err := clientcmd.Write(*cfg)
			if err != nil {
				return err
			}
			_, err = a.stdout.Write(b)
			return err
		},
	}
	return cmd
}

func (a *application) checkCmd() *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:               "check [context]...",
		Short:             "Проверить, что API отвечает",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: a.completeContexts,
		RunE: func(cmd *cobra.Command, args []string) error {
			names := args
			if len(names) == 0 {
				cur, err := kube.Current(a.resolvePath())
				if err != nil {
					return err
				}
				if cur == "" {
					return failf("укажите контекст или сделайте use", "нечего проверять")
				}
				names = []string{cur}
			}
			var failed bool
			results := make([]kube.CheckResult, 0, len(names))
			for _, n := range names {
				cr := kube.Check(a.resolvePath(), n, timeout)
				results = append(results, cr)
				if !cr.OK {
					failed = true
				}
			}
			if a.jsonOut {
				return json.NewEncoder(a.stdout).Encode(results)
			}
			for _, cr := range results {
				if cr.OK {
					fmt.Fprintf(a.stdout, "%s\tok\t%s\t%dms\n", cr.Name, cr.Version, cr.Latency)
				} else {
					fmt.Fprintf(a.stdout, "%s\tfail\t%s\t%dms\n", cr.Name, cr.Error, cr.Latency)
				}
			}
			if failed {
				return failf("", "check failed")
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "таймаут на контекст")
	return cmd
}

func (a *application) favCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "fav [context]",
		Short:             "Список или переключение избранного",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: a.completeContexts,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				list, err := recent.Favorites()
				if err != nil {
					return err
				}
				for _, n := range list {
					fmt.Fprintln(a.stdout, n)
				}
				return nil
			}
			on, err := recent.ToggleFavorite(args[0])
			if err != nil {
				return err
			}
			if on {
				fmt.Fprintf(a.stderr, "favorited %s\n", args[0])
			} else {
				fmt.Fprintf(a.stderr, "unfavorited %s\n", args[0])
			}
			fmt.Fprintln(a.stdout, args[0])
			return nil
		},
	}
	return cmd
}
