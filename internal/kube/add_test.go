package kube

import (
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestAddViaKUBECONFIGDoesNotWriteSource(t *testing.T) {
	dir := t.TempDir()
	dest := writeCfg(t, dir, "dest", baseDest(t))
	src := writeCfg(t, dir, "src", oneContextSrc(t))

	t.Setenv("KUBECONFIG", dest)
	res, err := Add(AddOptions{
		DestPath:   "", // resolve via KUBECONFIG → ModifyConfig path
		SourcePath: src,
		Client:     "acme",
		Name:       "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 1 || res.Added[0] != "acme/prod" {
		t.Fatalf("added: %#v", res.Added)
	}

	gotDest, err := clientcmd.LoadFromFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := gotDest.Contexts["acme/prod"]; !ok {
		t.Fatal("destination missing acme/prod")
	}

	gotSrc, err := clientcmd.LoadFromFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := gotSrc.Contexts["acme/prod"]; ok {
		t.Fatal("source was modified with destination context")
	}
	if _, ok := gotSrc.Contexts["admin@cluster"]; !ok {
		t.Fatal("source lost original context")
	}
}

func writeCfg(t *testing.T, dir, name string, cfg *clientcmdapi.Config) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func baseDest(t *testing.T) *clientcmdapi.Config {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["existing"] = &clientcmdapi.Cluster{Server: "https://existing.example"}
	cfg.AuthInfos["existing"] = &clientcmdapi.AuthInfo{Token: "old"}
	cfg.Contexts["other/keep"] = &clientcmdapi.Context{Cluster: "existing", AuthInfo: "existing"}
	cfg.CurrentContext = "other/keep"
	return cfg
}

func oneContextSrc(t *testing.T) *clientcmdapi.Config {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["c1"] = &clientcmdapi.Cluster{Server: "https://new.example"}
	cfg.AuthInfos["u1"] = &clientcmdapi.AuthInfo{Token: "tok"}
	cfg.Contexts["admin@cluster"] = &clientcmdapi.Context{Cluster: "c1", AuthInfo: "u1", Namespace: "ops"}
	return cfg
}

func TestAddMergesAndRenames(t *testing.T) {
	dir := t.TempDir()
	dest := writeCfg(t, dir, "dest", baseDest(t))
	src := writeCfg(t, dir, "src", oneContextSrc(t))

	res, err := Add(AddOptions{
		DestPath:   dest,
		SourcePath: src,
		Client:     "acme",
		Name:       "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 1 || res.Added[0] != "acme/prod" {
		t.Fatalf("added: %#v", res.Added)
	}

	got, err := clientcmd.LoadFromFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	ctx, ok := got.Contexts["acme/prod"]
	if !ok {
		t.Fatal("missing acme/prod")
	}
	if ctx.Namespace != "ops" || ctx.Cluster != "acme/prod" || ctx.AuthInfo != "acme/prod" {
		t.Fatalf("ctx: %+v", ctx)
	}
	if _, ok := got.Contexts["other/keep"]; !ok {
		t.Fatal("lost existing context")
	}
	if got.CurrentContext != "other/keep" {
		t.Fatalf("current changed unexpectedly: %s", got.CurrentContext)
	}
	if got.Clusters["acme/prod"].Server != "https://new.example" {
		t.Fatalf("cluster: %+v", got.Clusters["acme/prod"])
	}
}

func TestAddUseAndForceAndDryRun(t *testing.T) {
	dir := t.TempDir()
	dest := writeCfg(t, dir, "dest", baseDest(t))
	src := writeCfg(t, dir, "src", oneContextSrc(t))

	_, err := Add(AddOptions{DestPath: dest, SourcePath: src, Client: "acme", Name: "prod", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := clientcmd.LoadFromFile(dest)
	if _, ok := got.Contexts["acme/prod"]; ok {
		t.Fatal("dry-run wrote context")
	}

	if _, err := Add(AddOptions{DestPath: dest, SourcePath: src, Client: "acme", Name: "prod", Use: true}); err != nil {
		t.Fatal(err)
	}
	got, _ = clientcmd.LoadFromFile(dest)
	if got.CurrentContext != "acme/prod" {
		t.Fatalf("current: %s", got.CurrentContext)
	}

	if _, err := Add(AddOptions{DestPath: dest, SourcePath: src, Client: "acme", Name: "prod"}); err == nil {
		t.Fatal("expected collision error")
	}
	if _, err := Add(AddOptions{DestPath: dest, SourcePath: src, Client: "acme", Name: "prod", Force: true}); err != nil {
		t.Fatal(err)
	}
}

func TestAddInfersContextFromClusterUser(t *testing.T) {
	dir := t.TempDir()
	dest := writeCfg(t, dir, "dest", baseDest(t))
	srcCfg := clientcmdapi.NewConfig()
	srcCfg.Clusters["eks"] = &clientcmdapi.Cluster{Server: "https://eks.example"}
	srcCfg.AuthInfos["eks-user"] = &clientcmdapi.AuthInfo{Token: "tok"}
	src := writeCfg(t, dir, "src", srcCfg)

	res, err := Add(AddOptions{DestPath: dest, SourcePath: src, Client: "superform", Name: "eks"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Added[0] != "superform/eks" {
		t.Fatalf("added: %+v", res.Added)
	}
}

func TestAddRejectsBadClient(t *testing.T) {
	dir := t.TempDir()
	dest := writeCfg(t, dir, "dest", baseDest(t))
	src := writeCfg(t, dir, "src", oneContextSrc(t))
	if _, err := Add(AddOptions{DestPath: dest, SourcePath: src, Client: "a/b"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestAddMissingSource(t *testing.T) {
	dir := t.TempDir()
	dest := writeCfg(t, dir, "dest", baseDest(t))
	_, err := Add(AddOptions{DestPath: dest, SourcePath: filepath.Join(dir, "nope"), Client: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}
