package inventory

import (
	"reflect"
	"testing"
)

// The expectations here were MEASURED against ansible-core 2.21.4
// (ansible-playbook -i <inv> with a play whose hosts: is the pattern,
// reading the [WARNING] lines it wrote to stderr), not derived from
// reading its source. The source was then read to learn the rule
// behind them: _enumerate_matches warns when a term produced no hosts
// AND matched no group AND is not "all".
func TestMatchReportUnmatchedTerms(t *testing.T) {
	// good.ini in the measurement: one host, no groups of its own.
	populated := func() *Inventory {
		inv := New()
		inv.AddHost("h1", nil)
		return inv
	}
	// grp.ini in the measurement: a host plus a group with no hosts.
	withEmptyGroup := func() *Inventory {
		inv, err := ParseINI([]byte("h1\n\n[emptygrp]\n"))
		if err != nil {
			t.Fatalf("ParseINI: %v", err)
		}
		return inv
	}

	cases := []struct {
		name    string
		inv     *Inventory
		pattern string
		want    []string
	}{
		// Measured: two warnings, one per term, in written order.
		{"comma union, empty inventory", New(), "aaa,bbb", []string{"aaa", "bbb"}},
		// Measured: the term is reported WITHOUT its operator.
		{"negated term", populated(), "all:!zzz", []string{"zzz"}},
		{"intersected term", populated(), "all:&yyy", []string{"yyy"}},
		// Measured: no warning at all — an empty group is not a typo.
		{"empty group", withEmptyGroup(), "emptygrp", nil},
		// Measured: "all" against an empty inventory warns nothing
		// (real exempts it by name).
		{"all, empty inventory", New(), "all", nil},
		// Measured: hosts: localhost on an empty inventory runs the
		// play and warns nothing — the implicit localhost matched.
		{"implicit localhost", New(), "localhost", nil},
		{"ordinary hit", populated(), "h1", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, unmatched, err := tc.inv.MatchReport(tc.pattern)
			if err != nil {
				t.Fatalf("MatchReport(%q): %v", tc.pattern, err)
			}
			if !reflect.DeepEqual(unmatched, tc.want) {
				t.Errorf("MatchReport(%q) unmatched = %#v, want %#v", tc.pattern, unmatched, tc.want)
			}
		})
	}
}

// The reported order is real's EVALUATION order (plain terms, then &,
// then !), not the order the terms were written — orderPatterns, which
// Match has always applied to the matching itself.
func TestMatchReportUsesEvaluationOrder(t *testing.T) {
	inv := New()
	inv.AddHost("h1", nil)
	_, unmatched, err := inv.MatchReport("!zzz:&yyy:aaa")
	if err != nil {
		t.Fatalf("MatchReport: %v", err)
	}
	want := []string{"aaa", "yyy", "zzz"}
	if !reflect.DeepEqual(unmatched, want) {
		t.Fatalf("unmatched = %#v, want %#v", unmatched, want)
	}
}

// Match must keep behaving exactly as before: MatchReport is the same
// walk, and a caller that does not want the terms should see no change.
func TestMatchAgreesWithMatchReport(t *testing.T) {
	inv := New()
	inv.AddHost("h1", nil, "web")
	inv.AddHost("h2", nil, "db")
	for _, p := range []string{"all", "web", "web,db", "all:!web", "all:&db", "nosuch", ""} {
		a, err := inv.Match(p)
		if err != nil {
			t.Fatalf("Match(%q): %v", p, err)
		}
		b, _, err := inv.MatchReport(p)
		if err != nil {
			t.Fatalf("MatchReport(%q): %v", p, err)
		}
		if !reflect.DeepEqual(a, b) {
			t.Errorf("Match(%q) and MatchReport(%q) disagree: %v vs %v", p, p, a, b)
		}
	}
}
