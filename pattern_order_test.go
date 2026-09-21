package inventory

import (
	"strings"
	"testing"
)

// TestPatternOrderingMatchesRealAnsible pins host-pattern evaluation to
// results measured from real ansible-core 2.21.4. Real Ansible applies
// the terms of a pattern by KIND rather than left to right
// (order_patterns in ansible/inventory/manager.py): plain terms first,
// then `&`, then `!`.
//
// The first case is why this matters: applying terms in written order
// makes `!h1` select h1 — the exact inverse of what it means. A play
// written to avoid one machine ran on that machine and nowhere else.
func TestPatternOrderingMatchesRealAnsible(t *testing.T) {
	inv := fiveHosts(t)
	tests := []struct {
		pattern string
		want    string
	}{
		{"!h1", "h2,h3,h4,h5"},
		{"!h1:!h2", "h3,h4,h5"},
		{"web:!h1", "h2,h3,h4,h5"},
		// A plain term written AFTER an exclusion is still applied
		// before it, so it cannot resurrect an excluded host.
		{"!h1:web", "h2,h3,h4,h5"},
		{"web:!h1:h1", "h2,h3,h4,h5"},
		{"&web", "h1,h2,h3,h4,h5"},
		{"&web:!h1", "h2,h3,h4,h5"},
		{"h1:h2:!h2", "h1"},
		{"h1", "h1"},
		{"h1,h2", "h1,h2"},
		{"h1:h2", "h1,h2"},
		{"web", "h1,h2,h3,h4,h5"},
		{"all", "h1,h2,h3,h4,h5"},
		{"h*", "h1,h2,h3,h4,h5"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			hosts, err := inv.Match(tt.pattern)
			if err != nil {
				t.Fatal(err)
			}
			names := make([]string, 0, len(hosts))
			for _, h := range hosts {
				names = append(names, h.Name)
			}
			if got := strings.Join(names, ","); got != tt.want {
				t.Errorf("Match(%q) = %q, real ansible-core gives %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func fiveHosts(t *testing.T) *Inventory {
	t.Helper()
	inv, err := ParseYAML([]byte(`
web:
  hosts:
    h1: {ansible_connection: local}
    h2: {ansible_connection: local}
    h3: {ansible_connection: local}
    h4: {ansible_connection: local}
    h5: {ansible_connection: local}
`))
	if err != nil {
		t.Fatal(err)
	}
	return inv
}
