package inventory

import (
	"reflect"
	"testing"
)

// An inventory that parsed nothing is not an inventory with no
// structure. Measured against ansible-core 2.21.4 with an unreadable
// source:
//
//	ansible-inventory --list  -> "all": {"children": ["ungrouped"]}
//	ansible-inventory --graph -> "@all:" then "  |--@ungrouped:"
//
// This port related the two only in finalize, which an empty fallback
// inventory never reaches.
func TestEmptyInventoryStillRelatesAllAndUngrouped(t *testing.T) {
	inv := New()
	if got, want := inv.Groups["all"].ChildNames(), []string{"ungrouped"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("all children = %v, want %v", got, want)
	}
	if _, ok := inv.Groups["ungrouped"].Parents["all"]; !ok {
		t.Fatal("ungrouped has no parent all")
	}
}

// The link must not be duplicated once finalize runs for real.
func TestUngroupedIsNotListedTwiceAfterParsing(t *testing.T) {
	inv, err := ParseINI([]byte("[web]\nh1\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := inv.Groups["all"].ChildNames()
	seen := map[string]int{}
	for _, n := range got {
		seen[n]++
	}
	if seen["ungrouped"] != 1 {
		t.Fatalf("all children = %v, ungrouped appears %d times, want once", got, seen["ungrouped"])
	}
	if want := []string{"ungrouped", "web"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("all children = %v, want %v", got, want)
	}
}
