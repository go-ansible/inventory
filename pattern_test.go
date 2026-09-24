package inventory

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestMatchKeepsInventoryOrder: real lists and RUNS hosts in the order
// the inventory introduced them — "order: inventory", the default,
// means exactly that. Matching sorted them by name instead, so a
// two-host play's transcript came out in the wrong order and, at one
// fork, so did its execution.
func TestMatchKeepsInventoryOrder(t *testing.T) {
	dir := t.TempDir()
	// Deliberately NOT alphabetical: zulu first, alpha last.
	if err := os.WriteFile(filepath.Join(dir, "inv.ini"),
		[]byte("zulu\nmike\n\n[web]\nalpha\nbravo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv, err := Load(filepath.Join(dir, "inv.ini"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		pattern string
		want    []string
	}{
		{"all", []string{"zulu", "mike", "alpha", "bravo"}},
		{"web", []string{"alpha", "bravo"}},
	} {
		hosts, err := inv.Match(tc.pattern)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, h := range hosts {
			got = append(got, h.Name)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Match(%q) = %v, want %v", tc.pattern, got, tc.want)
		}
	}
}

// TestImplicitLocalhost pins the host real invents when the inventory
// does not define it, so `ansible localhost -m ping` and a play
// written `hosts: localhost` work with no inventory at all. This port
// had none, and answered "pattern localhost matched no hosts".
//
// Every rule here was measured: the three spellings, the
// case-sensitivity, the local connection, and that "all" does NOT
// match it — real even warns about exactly that.
func TestImplicitLocalhost(t *testing.T) {
	inv := New() // nothing defined at all

	for _, name := range []string{"localhost", "127.0.0.1", "::1"} {
		hosts, err := inv.Match(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(hosts) != 1 || hosts[0].Name != name {
			t.Errorf("Match(%q) = %v, want the implicit host", name, hosts)
			continue
		}
		if got := inv.HostVars(name)["ansible_connection"]; got != "local" {
			t.Errorf("%s: ansible_connection = %v, want local", name, got)
		}
	}
	// Case-sensitive: LOCALHOST gets nothing.
	if hosts, _ := inv.Match("LOCALHOST"); len(hosts) != 0 {
		t.Errorf(`Match("LOCALHOST") = %v, want nothing`, hosts)
	}
	// And "all" does not reach it.
	if hosts, _ := inv.Match("all"); len(hosts) != 0 {
		t.Errorf(`Match("all") = %v, want nothing — the implicit localhost is in no group`, hosts)
	}
}

// TestDefinedLocalhostWins: when the inventory DOES define localhost,
// its own entry is used, variables and all — the implicit one is only
// a fallback.
func TestDefinedLocalhostWins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inv.ini"),
		[]byte("localhost ansible_connection=ssh custom=yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv, err := Load(filepath.Join(dir, "inv.ini"))
	if err != nil {
		t.Fatal(err)
	}
	vars := inv.HostVars("localhost")
	if vars["ansible_connection"] != "ssh" {
		t.Errorf("ansible_connection = %v, want the inventory's own ssh", vars["ansible_connection"])
	}
	// An INI host var keeps its text: measured, "yes" stays a string
	// there (unlike the same word in a YAML file).
	if vars["custom"] != "yes" {
		t.Errorf("custom = %#v, want the inventory's own value", vars["custom"])
	}
	// A defined localhost IS in the inventory, so "all" reaches it.
	if hosts, _ := inv.Match("all"); len(hosts) != 1 {
		t.Errorf(`Match("all") = %v, want the defined localhost`, hosts)
	}
}
