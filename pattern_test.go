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
