package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestAddJSONCheckFailedExit(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "dest")
	src := filepath.Join(dir, "src")

	dstCfg := clientcmdapi.NewConfig()
	dstCfg.Clusters["keep"] = &clientcmdapi.Cluster{Server: "https://keep.example"}
	dstCfg.AuthInfos["keep"] = &clientcmdapi.AuthInfo{Token: "t"}
	dstCfg.Contexts["keep/x"] = &clientcmdapi.Context{Cluster: "keep", AuthInfo: "keep"}
	if err := clientcmd.WriteToFile(*dstCfg, dest); err != nil {
		t.Fatal(err)
	}

	srcCfg := clientcmdapi.NewConfig()
	srcCfg.Clusters["c"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:1"}
	srcCfg.AuthInfos["c"] = &clientcmdapi.AuthInfo{Token: "t"}
	srcCfg.Contexts["dead"] = &clientcmdapi.Context{Cluster: "c", AuthInfo: "c"}
	if err := clientcmd.WriteToFile(*srcCfg, src); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--kubeconfig", dest, "--json", "add", src, "--client", "acme", "--name", "dead", "--check"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		os.Getenv,
	)
	if code == 0 {
		t.Fatalf("want non-zero exit, got 0; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"ok":false`) {
		t.Fatalf("want ok=false in stdout, got %q", out)
	}
}
