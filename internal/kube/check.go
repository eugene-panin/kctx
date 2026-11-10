package kube

import (
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// CheckResult is a reachability probe for one context.
type CheckResult struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
	Latency int64  `json:"latency_ms"`
}

// Check talks to the API server of contextName (/version).
func Check(explicitPath, contextName string, timeout time.Duration) CheckResult {
	res := CheckResult{Name: contextName}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	start := time.Now()
	defer func() { res.Latency = time.Since(start).Milliseconds() }()

	cfg, err := loadingRules(explicitPath).Load()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if contextName == "" {
		contextName = cfg.CurrentContext
		res.Name = contextName
	}
	if contextName == "" {
		res.Error = "current-context не задан"
		return res
	}
	if _, ok := cfg.Contexts[contextName]; !ok {
		res.Error = fmt.Sprintf("context %q not found", contextName)
		return res
	}

	raw, err := clientcmd.NewDefaultClientConfig(*cfg, &clientcmd.ConfigOverrides{
		CurrentContext: contextName,
	}).ClientConfig()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	raw.Timeout = timeout

	client, err := kubernetes.NewForConfig(raw)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	info, err := client.Discovery().ServerVersion()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	res.Version = info.GitVersion
	return res
}
