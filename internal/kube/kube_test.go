package kube

import "testing"

func TestSplitClient(t *testing.T) {
	tests := []struct {
		name       string
		wantClient string
		wantShort  string
	}{
		{"matchsystems.com/dc3", "matchsystems.com", "dc3"},
		{"infra-car/l1", "infra-car", "l1"},
		{"kind-cka-lab", "kind-cka-lab", "kind-cka-lab"},
		{"a/b/c", "a", "b/c"},
		{"/only", "/only", "/only"},
		{"trailing/", "trailing/", "trailing/"},
	}
	for _, tt := range tests {
		client, short := SplitClient(tt.name)
		if client != tt.wantClient || short != tt.wantShort {
			t.Fatalf("SplitClient(%q) = %q, %q; want %q, %q",
				tt.name, client, short, tt.wantClient, tt.wantShort)
		}
	}
}

func TestFilter(t *testing.T) {
	entries := []Entry{
		{Name: "matchsystems.com/dc3", Client: "matchsystems.com", Short: "dc3"},
		{Name: "matchsystems.com/do1", Client: "matchsystems.com", Short: "do1"},
		{Name: "infra-car/l1", Client: "infra-car", Short: "l1"},
		{Name: "kind-cka-lab", Client: "kind-cka-lab", Short: "kind-cka-lab"},
	}
	got := Filter(entries, "match")
	if len(got) != 2 {
		t.Fatalf("Filter match: got %d, want 2", len(got))
	}
	got = Filter(entries, "l1")
	if len(got) != 1 || got[0].Name != "infra-car/l1" {
		t.Fatalf("Filter l1: %+v", got)
	}
	got = Filter(entries, "")
	if len(got) != 4 {
		t.Fatalf("Filter empty: got %d", len(got))
	}
}

func TestClients(t *testing.T) {
	entries := []Entry{
		{Client: "a"},
		{Client: "a"},
		{Client: "b"},
	}
	got := Clients(entries)
	if len(got) != 2 || got[0].Name != "a" || got[0].Count != 2 || got[1].Count != 1 {
		t.Fatalf("Clients: %+v", got)
	}
}
