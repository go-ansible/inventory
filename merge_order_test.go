package inventory

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Loading a FILE parses it and then Merges the result into the empty
// inventory Load starts from, so every order Merge does not preserve is
// lost on the way out -- which is why parsing alone passing its own
// order tests proved nothing about Load.
//
// Measured with ansible-inventory --list against ansible-core 2.21.4
// on this exact file: "all": {"children": ["ungrouped", "web",
// "empty"]} -- creation order, web before empty because [web] is
// written first.
const mergeOrderINI = `[web]
h1
h2

[empty]

[empty:vars]
k=v
`

func TestLoadPreservesGroupOrderThroughMerge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.ini")
	if err := os.WriteFile(path, []byte(mergeOrderINI), 0o644); err != nil {
		t.Fatal(err)
	}
	// Many times: the old behaviour ranged a map, so it was random
	// per call rather than merely wrong.
	want := []string{"ungrouped", "web", "empty"}
	for i := 0; i < 200; i++ {
		inv, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got := inv.Groups["all"].ChildNames(); !reflect.DeepEqual(got, want) {
			t.Fatalf("load %d: all children = %v, want %v", i, got, want)
		}
	}
}

// Merge must preserve a group's OWN children order too, not just the
// order groups appear in.
func TestMergePreservesChildOrder(t *testing.T) {
	src, err := ParseINI([]byte("[web]\nh1\n\n[db]\nh2\n\n[prod:children]\nweb\ndb\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"web", "db"}
	for i := 0; i < 200; i++ {
		dst := New()
		dst.Merge(src)
		if got := dst.Groups["prod"].ChildNames(); !reflect.DeepEqual(got, want) {
			t.Fatalf("merge %d: prod children = %v, want %v", i, got, want)
		}
	}
}

// And the host order inside a group, which the dump also reports.
func TestMergePreservesGroupHostOrder(t *testing.T) {
	src, err := ParseINI([]byte("[web]\nzz\naa\nmm\n"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		dst := New()
		dst.Merge(src)
		var got []string
		for _, n := range dst.hostOrder() {
			if _, ok := dst.Groups["web"].Hosts[n]; ok {
				got = append(got, n)
			}
		}
		if want := []string{"zz", "aa", "mm"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("merge %d: web hosts = %v, want %v", i, got, want)
		}
	}
}
