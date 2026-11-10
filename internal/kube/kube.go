package kube

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
)

// Entry is one kubeconfig context, grouped by client for display.
type Entry struct {
	Name      string
	Client    string
	Short     string
	Namespace string
	Current   bool
}

// SplitClient takes the part before the first '/' as client.
// Names without '/' are their own client group.
func SplitClient(name string) (client, short string) {
	client, short, ok := strings.Cut(name, "/")
	if !ok || client == "" || short == "" {
		return name, name
	}
	return client, short
}

// Load reads contexts from kubeconfig (KUBECONFIG / explicit path / default).
func Load(explicitPath string) ([]Entry, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if explicitPath != "" {
		rules.ExplicitPath = explicitPath
	}
	cfg, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	out := make([]Entry, 0, len(cfg.Contexts))
	for name, ctx := range cfg.Contexts {
		if ctx == nil {
			continue
		}
		client, short := SplitClient(name)
		out = append(out, Entry{
			Name:      name,
			Client:    client,
			Short:     short,
			Namespace: ctx.Namespace,
			Current:   name == cfg.CurrentContext,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Client != out[j].Client {
			return out[i].Client < out[j].Client
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Current returns the current context name, or empty if unset.
func Current(explicitPath string) (string, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if explicitPath != "" {
		rules.ExplicitPath = explicitPath
	}
	cfg, err := rules.Load()
	if err != nil {
		return "", fmt.Errorf("load kubeconfig: %w", err)
	}
	return cfg.CurrentContext, nil
}

// Use sets current-context to name.
func Use(explicitPath, name string) error {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if explicitPath != "" {
		rules.ExplicitPath = explicitPath
	}
	cfg, err := rules.Load()
	if err != nil {
		return fmt.Errorf("load kubeconfig: %w", err)
	}
	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("context %q not found", name)
	}
	cfg.CurrentContext = name
	if err := clientcmd.ModifyConfig(rules, *cfg, true); err != nil {
		return fmt.Errorf("set current-context: %w", err)
	}
	return nil
}

// Clients returns unique client names in sorted order with counts.
func Clients(entries []Entry) []ClientCount {
	counts := map[string]int{}
	order := make([]string, 0)
	for _, e := range entries {
		if _, ok := counts[e.Client]; !ok {
			order = append(order, e.Client)
		}
		counts[e.Client]++
	}
	out := make([]ClientCount, 0, len(order))
	for _, c := range order {
		out = append(out, ClientCount{Name: c, Count: counts[c]})
	}
	return out
}

// ClientCount is a client group summary.
type ClientCount struct {
	Name  string
	Count int
}

// Filter keeps entries whose client or name fuzzy-matches query (substring, case-insensitive).
func Filter(entries []Entry, query string) []Entry {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return entries
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		hay := strings.ToLower(e.Client + " " + e.Name + " " + e.Short + " " + e.Namespace)
		if strings.Contains(hay, query) {
			out = append(out, e)
			continue
		}
		// also match contiguous runes across client/short
		if strings.Contains(strings.ToLower(e.Name), query) {
			out = append(out, e)
		}
	}
	return out
}
