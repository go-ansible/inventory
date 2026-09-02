package inventory

import "testing"

func TestAddHostReachableByAllPattern(t *testing.T) {
	inv := New()
	inv.AddHost("dynamic1", map[string]any{"ansible_host": "10.0.0.1"})

	hosts, err := inv.Match("all")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hosts {
		if h.Name == "dynamic1" {
			found = true
		}
	}
	if !found {
		t.Fatal("dynamic1 not reachable by the all pattern")
	}
	if got := inv.HostVars("dynamic1")["ansible_host"]; got != "10.0.0.1" {
		t.Fatalf("ansible_host = %v", got)
	}
}

func TestAddHostWithGroups(t *testing.T) {
	inv := New()
	inv.AddHost("dynamic1", nil, "web", "prod")

	hosts, err := inv.Match("web")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Name != "dynamic1" {
		t.Fatalf("web group = %v", hosts)
	}
	hosts, err = inv.Match("prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Name != "dynamic1" {
		t.Fatalf("prod group = %v", hosts)
	}
}

func TestAddHostVarsMergeIntoExisting(t *testing.T) {
	inv := New()
	inv.AddHost("h1", map[string]any{"a": 1})
	inv.AddHost("h1", map[string]any{"b": 2})

	vars := inv.HostVars("h1")
	if vars["a"] != 1 || vars["b"] != 2 {
		t.Fatalf("vars = %v", vars)
	}
}

func TestAddToGroupExistingHost(t *testing.T) {
	inv := New()
	inv.AddHost("h1", nil)
	inv.AddToGroup("h1", "newgroup")

	hosts, err := inv.Match("newgroup")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Name != "h1" {
		t.Fatalf("newgroup = %v", hosts)
	}
}

func TestAddToGroupCreatesNewHost(t *testing.T) {
	inv := New()
	inv.AddToGroup("brandnew", "g1")

	hosts, err := inv.Match("g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Name != "brandnew" {
		t.Fatalf("g1 = %v", hosts)
	}
}

func TestAddHostGroupedNotUngrouped(t *testing.T) {
	inv := New()
	inv.AddHost("h1", nil, "web")

	ungrouped, err := inv.Match("ungrouped")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range ungrouped {
		if h.Name == "h1" {
			t.Fatal("h1 should not be in ungrouped once assigned to web")
		}
	}
}
