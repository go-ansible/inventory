package inventory

import (
	"reflect"
	"testing"
)

const orderINI = `[web]
h1
h2

[db]
h3

[prod:children]
web
db
`

// Measured with ansible-inventory -i <the same file> --list against
// ansible-core 2.21.4:
//
//	"all":  {"children": ["ungrouped", "prod"]}
//	"prod": {"children": ["web", "db"]}
//
// Neither is alphabetical. "prod" lists its children in the order the
// [prod:children] section wrote them, and "all" lists ungrouped first
// because it exists from the start.
func TestChildOrderIsDocumentOrder(t *testing.T) {
	inv, err := ParseINI([]byte(orderINI))
	if err != nil {
		t.Fatalf("ParseINI: %v", err)
	}
	if got, want := inv.Groups["prod"].ChildNames(), []string{"web", "db"}; !reflect.DeepEqual(got, want) {
		t.Errorf("prod children = %v, want %v (document order, not alphabetical)", got, want)
	}
	if got, want := inv.Groups["all"].ChildNames(), []string{"ungrouped", "prod"}; !reflect.DeepEqual(got, want) {
		t.Errorf("all children = %v, want %v", got, want)
	}
}

// The order used to come from ranging over a Go map, so it was random
// per process AND per call. One parse cannot show that; many can.
func TestChildOrderIsDeterministic(t *testing.T) {
	first, err := ParseINI([]byte(orderINI))
	if err != nil {
		t.Fatal(err)
	}
	wantAll := first.Groups["all"].ChildNames()
	wantProd := first.Groups["prod"].ChildNames()
	for i := 0; i < 200; i++ {
		inv, err := ParseINI([]byte(orderINI))
		if err != nil {
			t.Fatal(err)
		}
		if got := inv.Groups["all"].ChildNames(); !reflect.DeepEqual(got, wantAll) {
			t.Fatalf("parse %d: all children = %v, want %v", i, got, wantAll)
		}
		if got := inv.Groups["prod"].ChildNames(); !reflect.DeepEqual(got, wantProd) {
			t.Fatalf("parse %d: prod children = %v, want %v", i, got, wantProd)
		}
	}
}

// ChildNames must not hand out the slice it keeps, or a caller sorting
// the result would reorder the inventory itself.
func TestChildNamesIsACopy(t *testing.T) {
	inv, err := ParseINI([]byte(orderINI))
	if err != nil {
		t.Fatal(err)
	}
	got := inv.Groups["prod"].ChildNames()
	got[0], got[1] = got[1], got[0]
	if again := inv.Groups["prod"].ChildNames(); !reflect.DeepEqual(again, []string{"web", "db"}) {
		t.Fatalf("mutating the result changed the group: %v", again)
	}
}

// A group added twice as a child must appear once, in its FIRST
// position -- add_host/group_by can re-add an existing child.
func TestChildOrderIgnoresDuplicates(t *testing.T) {
	inv := New()
	inv.addChild("p", "a")
	inv.addChild("p", "b")
	inv.addChild("p", "a")
	if got, want := inv.Groups["p"].ChildNames(), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
}
