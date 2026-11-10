package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"kctx/internal/kube"
	"kctx/internal/tui"
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

	root.AddCommand(a.lsCmd())
	root.AddCommand(a.useCmd())
	root.AddCommand(a.currentCmd())
	root.AddCommand(a.clientsCmd())
	return root
}

func (a *application) resolvePath() string {
	if a.kubeconfig != "" {
		return a.kubeconfig
	}
	return a.getenv("KUBECONFIG")
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
		return failf("добавьте контексты в kubeconfig", "контекстов нет")
	}
	res, err := tui.Run(entries)
	if err != nil {
		return err
	}
	if res.Canceled {
		return nil
	}
	if err := kube.Use(a.resolvePath(), res.Selected); err != nil {
		return err
	}
	fmt.Fprintln(a.stdout, res.Selected)
	fmt.Fprintf(a.stderr, "switched to %s\n", res.Selected)
	return nil
}

func (a *application) lsCmd() *cobra.Command {
	cmd := &cobra.Command{
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
	return cmd
}

func (a *application) useCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <context>",
		Short: "Сделать контекст текущим",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := kube.Use(a.resolvePath(), name); err != nil {
				return failf("проверьте имя: kctx ls", "%v", err)
			}
			fmt.Fprintln(a.stdout, name)
			fmt.Fprintf(a.stderr, "switched to %s\n", name)
			return nil
		},
	}
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
					"name":   name,
					"client": client,
					"short":  short,
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
