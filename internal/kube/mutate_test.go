package kube

import (
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestMoveAndNamespaceAndExport(t *testing.T) {
	dir := t.TempDir()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["old"] = &clientcmdapi.Cluster{Server: "https://x"}
	cfg.AuthInfos["old"] = &clientcmdapi.AuthInfo{Token: "t"}
	cfg.Contexts["unsorted/x"] = &clientcmdapi.Context{Cluster: "old", AuthInfo: "old", Namespace: "default"}
	cfg.CurrentContext = "unsorted/x"
	dest := writeCfg(t, dir, "dest", cfg)

	if err := Move(dest, "unsorted/x", "acme/prod"); err != nil {
		t.Fatal(err)
	}
	got, err := clientcmd.LoadFromFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Contexts["acme/prod"]; !ok {
		t.Fatal("missing acme/prod")
	}
	if _, ok := got.Contexts["unsorted/x"]; ok {
		t.Fatal("old context remains")
	}
	if got.CurrentContext != "acme/prod" {
		t.Fatalf("current: %s", got.CurrentContext)
	}
	if _, ok := got.Clusters["old"]; ok {
		t.Fatal("orphan cluster remains")
	}

	if err := SetNamespace(dest, "acme/prod", "ops"); err != nil {
		t.Fatal(err)
	}
	ns, err := Namespace(dest, "acme/prod")
	if err != nil || ns != "ops" {
		t.Fatalf("ns=%q err=%v", ns, err)
	}

	exp, err := Export(dest, []string{"acme/prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(exp.Contexts) != 1 || exp.CurrentContext != "acme/prod" {
		t.Fatalf("export: %+v", exp)
	}
}

func TestAddFromReaderSanitizes(t *testing.T) {
	dir := t.TempDir()
	dest := writeCfg(t, dir, "dest", baseDest(t))
	src := oneContextSrc(t) // admin@cluster
	data, err := clientcmd.Write(*src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Add(AddOptions{
		DestPath:   dest,
		SourcePath: "-",
		Source:     strings.NewReader(string(data)),
		Client:     "acme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Added[0] != "acme/admin-cluster" {
		t.Fatalf("want acme/admin-cluster, got %v (sanitize=%q)", res.Added, Sanitize("admin@cluster"))
	}
}
