package kube

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// AddOptions controls merging a source kubeconfig into the destination.
type AddOptions struct {
	DestPath   string // explicit destination; empty → default loading rules
	SourcePath string // file path, or "-" for stdin/Source
	Source     io.Reader
	Client     string // required: becomes the group prefix
	Name       string // optional short name; only when source yields one context
	Use        bool   // set current-context to the added name (single only)
	Force      bool   // overwrite an existing context with the same name
	DryRun     bool   // do not write
	Check      bool   // probe after add (caller runs Check; flag reserved for app)
}

// AddResult lists context names that were (or would be) added.
type AddResult struct {
	Added      []string
	Overwrites []string // existing names replaced with --force
}

// Add merges contexts from SourcePath into the destination kubeconfig,
// renaming them to client/short.
func Add(opts AddOptions) (AddResult, error) {
	if strings.TrimSpace(opts.Client) == "" {
		return AddResult{}, fmt.Errorf("client is required")
	}
	if strings.Contains(opts.Client, "/") || strings.ContainsAny(opts.Client, " \t") {
		return AddResult{}, fmt.Errorf("invalid client name %q", opts.Client)
	}

	src, err := loadSource(opts.SourcePath, opts.Source)
	if err != nil {
		return AddResult{}, err
	}
	if err := ensureSourceContexts(src); err != nil {
		return AddResult{}, err
	}

	srcNames := contextNames(src)
	if opts.Name != "" && len(srcNames) != 1 {
		return AddResult{}, fmt.Errorf("--name requires exactly one context in source (got %d)", len(srcNames))
	}
	if opts.Name != "" {
		if strings.Contains(opts.Name, "/") {
			return AddResult{}, fmt.Errorf("--name must not contain '/'")
		}
		opts.Name = Sanitize(opts.Name)
	}

	rules := loadingRules(opts.DestPath)
	dst, err := rules.Load()
	if err != nil {
		return AddResult{}, fmt.Errorf("load kubeconfig: %w", err)
	}
	if dst.Clusters == nil {
		dst.Clusters = map[string]*clientcmdapi.Cluster{}
	}
	if dst.AuthInfos == nil {
		dst.AuthInfos = map[string]*clientcmdapi.AuthInfo{}
	}
	if dst.Contexts == nil {
		dst.Contexts = map[string]*clientcmdapi.Context{}
	}

	added := make([]string, 0, len(srcNames))
	overwrites := make([]string, 0)
	for _, oldName := range srcNames {
		ctx := src.Contexts[oldName]
		if ctx == nil {
			continue
		}
		short := opts.Name
		if short == "" {
			short = Sanitize(oldName)
		}
		if short == "" {
			return AddResult{}, fmt.Errorf("empty short name for context %q", oldName)
		}
		newName := opts.Client + "/" + short
		if _, exists := dst.Contexts[newName]; exists {
			if !opts.Force {
				return AddResult{}, fmt.Errorf("context %q already exists (use --force to overwrite)", newName)
			}
			overwrites = append(overwrites, newName)
		}

		cluster, ok := src.Clusters[ctx.Cluster]
		if !ok || cluster == nil {
			return AddResult{}, fmt.Errorf("context %q references missing cluster %q", oldName, ctx.Cluster)
		}
		user, ok := src.AuthInfos[ctx.AuthInfo]
		if !ok || user == nil {
			return AddResult{}, fmt.Errorf("context %q references missing user %q", oldName, ctx.AuthInfo)
		}

		// Clear LocationOfOrigin so ModifyConfig writes into the destination
		// kubeconfig, not back into the source file the objects were loaded from.
		c := cluster.DeepCopy()
		c.LocationOfOrigin = ""
		u := user.DeepCopy()
		u.LocationOfOrigin = ""
		newCtx := ctx.DeepCopy()
		newCtx.LocationOfOrigin = ""
		newCtx.Cluster = newName
		newCtx.AuthInfo = newName
		dst.Clusters[newName] = c
		dst.AuthInfos[newName] = u
		dst.Contexts[newName] = newCtx
		added = append(added, newName)
	}

	if len(added) == 0 {
		return AddResult{}, fmt.Errorf("no contexts to add")
	}
	sort.Strings(added)
	sort.Strings(overwrites)

	if opts.Use {
		if len(added) != 1 {
			return AddResult{}, fmt.Errorf("--use requires exactly one added context (got %d)", len(added))
		}
		dst.CurrentContext = added[0]
	}

	if opts.DryRun {
		return AddResult{Added: added, Overwrites: overwrites}, nil
	}
	if err := writeConfig(rules, dst); err != nil {
		return AddResult{}, err
	}
	return AddResult{Added: added, Overwrites: overwrites}, nil
}

func loadSource(path string, r io.Reader) (*clientcmdapi.Config, error) {
	if path == "-" || r != nil && path == "" {
		if r == nil {
			r = os.Stdin
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("read source: %w", err)
		}
		cfg, err := clientcmd.Load(data)
		if err != nil {
			return nil, fmt.Errorf("load source: %w", err)
		}
		return cfg, nil
	}
	if path == "" {
		return nil, fmt.Errorf("source path is required")
	}
	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load source: %w", err)
	}
	return cfg, nil
}

func writeConfig(rules *clientcmd.ClientConfigLoadingRules, cfg *clientcmdapi.Config) error {
	path := rules.ExplicitPath
	if path == "" {
		path = rules.GetDefaultFilename()
	}
	if path == "" {
		return fmt.Errorf("write kubeconfig: empty destination path")
	}
	for _, c := range cfg.Clusters {
		if c != nil && c.LocationOfOrigin == "" {
			c.LocationOfOrigin = path
		}
	}
	for _, u := range cfg.AuthInfos {
		if u != nil && u.LocationOfOrigin == "" {
			u.LocationOfOrigin = path
		}
	}
	for _, c := range cfg.Contexts {
		if c != nil && c.LocationOfOrigin == "" {
			c.LocationOfOrigin = path
		}
	}
	if rules.ExplicitPath != "" {
		if err := clientcmd.WriteToFile(*cfg, path); err != nil {
			return fmt.Errorf("write kubeconfig: %w", err)
		}
		return nil
	}
	if err := clientcmd.ModifyConfig(rules, *cfg, true); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}
	return nil
}

func contextNames(cfg *clientcmdapi.Config) []string {
	names := make([]string, 0, len(cfg.Contexts))
	for name, ctx := range cfg.Contexts {
		if ctx != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// ensureSourceContexts creates a single context when the file has cluster+user
// but an empty contexts list (common in partial exports).
func ensureSourceContexts(cfg *clientcmdapi.Config) error {
	if len(contextNames(cfg)) > 0 {
		return nil
	}
	clusters := make([]string, 0, len(cfg.Clusters))
	for name, c := range cfg.Clusters {
		if c != nil {
			clusters = append(clusters, name)
		}
	}
	users := make([]string, 0, len(cfg.AuthInfos))
	for name, u := range cfg.AuthInfos {
		if u != nil {
			users = append(users, name)
		}
	}
	sort.Strings(clusters)
	sort.Strings(users)
	if len(clusters) == 1 && len(users) == 1 {
		name := clusters[0]
		if cfg.Contexts == nil {
			cfg.Contexts = map[string]*clientcmdapi.Context{}
		}
		cfg.Contexts[name] = &clientcmdapi.Context{
			Cluster:  clusters[0],
			AuthInfo: users[0],
		}
		return nil
	}
	return fmt.Errorf("source has no contexts (and not exactly one cluster+user to infer)")
}
