package kube

import (
	"fmt"
	"strings"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Move renames a context to client/short. Cluster and user keys follow the new
// name; the old context (and orphaned keys) are removed. The returned string is
// the name actually written (extra '/' in the short part become '-').
func Move(explicitPath, from, to string) (string, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return "", fmt.Errorf("from and to are required")
	}
	if !strings.Contains(to, "/") {
		return "", fmt.Errorf("destination must be client/cluster, got %q", to)
	}
	client, short := SplitClient(to)
	if client == "" || short == "" || strings.Contains(client+"/", "//") {
		return "", fmt.Errorf("invalid destination %q", to)
	}
	// keep only first slash as client separator
	if i := strings.IndexByte(to, '/'); i >= 0 {
		to = to[:i+1] + strings.ReplaceAll(to[i+1:], "/", "-")
	}

	rules := loadingRules(explicitPath)
	cfg, err := rules.Load()
	if err != nil {
		return "", fmt.Errorf("load kubeconfig: %w", err)
	}
	ctx, ok := cfg.Contexts[from]
	if !ok || ctx == nil {
		return "", fmt.Errorf("context %q not found", from)
	}
	if from != to {
		if _, exists := cfg.Contexts[to]; exists {
			return "", fmt.Errorf("context %q already exists", to)
		}
	}

	cluster, ok := cfg.Clusters[ctx.Cluster]
	if !ok || cluster == nil {
		return "", fmt.Errorf("context %q references missing cluster %q", from, ctx.Cluster)
	}
	user, ok := cfg.AuthInfos[ctx.AuthInfo]
	if !ok || user == nil {
		return "", fmt.Errorf("context %q references missing user %q", from, ctx.AuthInfo)
	}

	oldCluster, oldUser := ctx.Cluster, ctx.AuthInfo
	c := cluster.DeepCopy()
	c.LocationOfOrigin = ""
	u := user.DeepCopy()
	u.LocationOfOrigin = ""
	newCtx := ctx.DeepCopy()
	newCtx.LocationOfOrigin = ""
	newCtx.Cluster = to
	newCtx.AuthInfo = to
	cfg.Clusters[to] = c
	cfg.AuthInfos[to] = u
	cfg.Contexts[to] = newCtx
	if from != to {
		delete(cfg.Contexts, from)
	}
	deleteUnused(cfg, oldCluster, oldUser)
	if cfg.CurrentContext == from {
		cfg.CurrentContext = to
	}
	if err := writeConfig(rules, cfg); err != nil {
		return "", err
	}
	return to, nil
}

func deleteUnused(cfg *clientcmdapi.Config, cluster, user string) {
	clusterUsed, userUsed := false, false
	for _, c := range cfg.Contexts {
		if c == nil {
			continue
		}
		if c.Cluster == cluster {
			clusterUsed = true
		}
		if c.AuthInfo == user {
			userUsed = true
		}
	}
	if !clusterUsed {
		delete(cfg.Clusters, cluster)
	}
	if !userUsed {
		delete(cfg.AuthInfos, user)
	}
}

// SetNamespace sets the default namespace of a context. Empty ns clears it.
func SetNamespace(explicitPath, contextName, ns string) error {
	if contextName == "" {
		return fmt.Errorf("context is required")
	}
	if ns != "" && !validDNSLabel(ns) {
		return fmt.Errorf("invalid namespace %q", ns)
	}
	rules := loadingRules(explicitPath)
	cfg, err := rules.Load()
	if err != nil {
		return fmt.Errorf("load kubeconfig: %w", err)
	}
	ctx, ok := cfg.Contexts[contextName]
	if !ok || ctx == nil {
		return fmt.Errorf("context %q not found", contextName)
	}
	ctx.Namespace = ns
	return writeConfig(rules, cfg)
}

// Namespace returns the default namespace of a context (or current if name empty).
func Namespace(explicitPath, contextName string) (string, error) {
	cfg, err := loadingRules(explicitPath).Load()
	if err != nil {
		return "", fmt.Errorf("load kubeconfig: %w", err)
	}
	if contextName == "" {
		contextName = cfg.CurrentContext
	}
	if contextName == "" {
		return "", fmt.Errorf("current-context не задан")
	}
	ctx, ok := cfg.Contexts[contextName]
	if !ok || ctx == nil {
		return "", fmt.Errorf("context %q not found", contextName)
	}
	return ctx.Namespace, nil
}

func validDNSLabel(s string) bool {
	if len(s) == 0 || len(s) > 63 {
		return false
	}
	for i := 0; i < len(s); i++ {
		r := s[i]
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if i > 0 && i < len(s)-1 && r == '-' {
			ok = true
		}
		if !ok {
			return false
		}
	}
	return true
}

// Export builds a self-contained kubeconfig with only the named contexts.
func Export(explicitPath string, names []string) (*clientcmdapi.Config, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("at least one context is required")
	}
	cfg, err := loadingRules(explicitPath).Load()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	out := clientcmdapi.NewConfig()
	for _, name := range names {
		ctx, ok := cfg.Contexts[name]
		if !ok || ctx == nil {
			return nil, fmt.Errorf("context %q not found", name)
		}
		cluster, ok := cfg.Clusters[ctx.Cluster]
		if !ok || cluster == nil {
			return nil, fmt.Errorf("context %q references missing cluster %q", name, ctx.Cluster)
		}
		user, ok := cfg.AuthInfos[ctx.AuthInfo]
		if !ok || user == nil {
			return nil, fmt.Errorf("context %q references missing user %q", name, ctx.AuthInfo)
		}
		out.Clusters[name] = cluster.DeepCopy()
		out.AuthInfos[name] = user.DeepCopy()
		newCtx := ctx.DeepCopy()
		newCtx.Cluster = name
		newCtx.AuthInfo = name
		out.Contexts[name] = newCtx
	}
	out.CurrentContext = names[0]
	return out, nil
}
