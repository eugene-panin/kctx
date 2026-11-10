package kube

import (
	"os"
	"path/filepath"
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

	gotName, err := Move(dest, "unsorted/x", "acme/prod")
	if err != nil {
		t.Fatal(err)
	}
	if gotName != "acme/prod" {
		t.Fatalf("Move name: %q", gotName)
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

func TestMoveSanitizesExtraSlash(t *testing.T) {
	dir := t.TempDir()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["old"] = &clientcmdapi.Cluster{Server: "https://x"}
	cfg.AuthInfos["old"] = &clientcmdapi.AuthInfo{Token: "t"}
	cfg.Contexts["unsorted/x"] = &clientcmdapi.Context{Cluster: "old", AuthInfo: "old"}
	dest := writeCfg(t, dir, "dest", cfg)

	got, err := Move(dest, "unsorted/x", "acme/prod/v2")
	if err != nil {
		t.Fatal(err)
	}
	if got != "acme/prod-v2" {
		t.Fatalf("got %q, want acme/prod-v2", got)
	}
	loaded, err := clientcmd.LoadFromFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Contexts["acme/prod-v2"]; !ok {
		t.Fatal("missing sanitized name in file")
	}
	if _, ok := loaded.Contexts["acme/prod/v2"]; ok {
		t.Fatal("raw multi-slash name should not exist")
	}
}

func TestExportInlinesFileCredentials(t *testing.T) {
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.crt")
	cert := filepath.Join(dir, "client.crt")
	key := filepath.Join(dir, "client.key")
	tok := filepath.Join(dir, "token")
	for path, body := range map[string]string{
		ca: "CA-DATA", cert: "CERT-DATA", key: "KEY-DATA", tok: "tok-secret",
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["c"] = &clientcmdapi.Cluster{
		Server:               "https://x",
		CertificateAuthority: ca,
		LocationOfOrigin:     filepath.Join(dir, "cfg"),
	}
	cfg.AuthInfos["c"] = &clientcmdapi.AuthInfo{
		ClientCertificate: cert,
		ClientKey:         key,
		TokenFile:         tok,
		LocationOfOrigin:  filepath.Join(dir, "cfg"),
	}
	cfg.Contexts["acme/prod"] = &clientcmdapi.Context{Cluster: "c", AuthInfo: "c"}
	dest := writeCfg(t, dir, "dest", cfg)

	out, err := Export(dest, []string{"acme/prod"})
	if err != nil {
		t.Fatal(err)
	}
	cl := out.Clusters["acme/prod"]
	us := out.AuthInfos["acme/prod"]
	if cl.CertificateAuthority != "" || string(cl.CertificateAuthorityData) != "CA-DATA" {
		t.Fatalf("ca not inlined: path=%q data=%q", cl.CertificateAuthority, cl.CertificateAuthorityData)
	}
	if us.ClientCertificate != "" || string(us.ClientCertificateData) != "CERT-DATA" {
		t.Fatalf("cert not inlined: %#v", us)
	}
	if us.ClientKey != "" || string(us.ClientKeyData) != "KEY-DATA" {
		t.Fatalf("key not inlined: %#v", us)
	}
	if us.TokenFile != "" || us.Token != "tok-secret" {
		t.Fatalf("token not inlined: token=%q file=%q", us.Token, us.TokenFile)
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
