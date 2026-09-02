package inventory

import "testing"

func TestMerge(t *testing.T) {
	inv := New()
	inv.AddHost("h1", map[string]any{"a": 1}, "web")
	inv.AddHost("h2", nil, "db")
	inv.group("web").Vars["x"] = 1

	other := New()
	other.AddHost("h1", map[string]any{"a": 2, "b": 3}, "web") // overrides a, adds b
	other.AddHost("h3", nil, "web")                            // union: new web member
	other.group("web").Vars["x"] = 2                           // overrides group var
	other.addChild("dc", "web")                                // new parent group, child link

	inv.Merge(other)

	hv := inv.HostVars("h1")
	if hv["a"] != 2 {
		t.Errorf("h1.a = %v, want 2 (later source wins)", hv["a"])
	}
	if hv["b"] != 3 {
		t.Errorf("h1.b = %v, want 3", hv["b"])
	}

	web := inv.Groups["web"]
	if web == nil {
		t.Fatal("web group missing after merge")
	}
	if web.Vars["x"] != 2 {
		t.Errorf("web.x = %v, want 2 (later source wins)", web.Vars["x"])
	}
	for _, want := range []string{"h1", "h3"} {
		if _, ok := web.Hosts[want]; !ok {
			t.Errorf("web group missing host %q after merge (union)", want)
		}
	}

	dc, ok := inv.Groups["dc"]
	if !ok {
		t.Fatal("dc group not created by merge")
	}
	if _, ok := dc.Children["web"]; !ok {
		t.Error("dc should have web as a child after merge")
	}

	// h2 stayed in db, untouched by other.
	if _, ok := inv.Groups["db"].Hosts["h2"]; !ok {
		t.Error("h2 should remain in db after merge")
	}

	// finalize should have re-run: h1/h3 are grouped, so not ungrouped.
	for _, name := range []string{"h1", "h3", "h2"} {
		if _, ok := inv.Groups["ungrouped"].Hosts[name]; ok {
			t.Errorf("%s should not be ungrouped", name)
		}
	}
}
