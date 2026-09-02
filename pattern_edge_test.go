package inventory

import (
	"reflect"
	"sort"
	"testing"
)

func TestMatchEmptyPattern(t *testing.T) {
	inv := New()
	inv.AddHost("h1", nil)
	hosts, err := inv.Match("")
	if err != nil {
		t.Fatalf("Match(\"\"): %v", err)
	}
	if hosts != nil {
		t.Fatalf("Match(\"\") = %v, want nil", hosts)
	}
}

func TestMatchErrorPropagation(t *testing.T) {
	inv := New()
	if _, err := inv.Match("web[1:5:0]"); err == nil {
		t.Fatal("Match with invalid range step: got nil error, want one")
	}
	if _, err := inv.Match("web[1:5"); err == nil {
		t.Fatal("Match with unbalanced '[': got nil error, want one")
	}
	if _, err := inv.Match("web]1:5["); err == nil {
		t.Fatal("Match with unbalanced ']': got nil error, want one")
	}
}

func TestMatchCommaUnion(t *testing.T) {
	inv := New()
	inv.AddHost("h1", nil, "web")
	inv.AddHost("h2", nil, "db")
	hosts, err := inv.Match("web,db")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	var got []string
	for _, h := range hosts {
		got = append(got, h.Name)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"h1", "h2"}) {
		t.Fatalf("Match(web,db) = %v, want [h1 h2]", got)
	}
}

func TestMatchWithSpacesAroundTerms(t *testing.T) {
	inv := New()
	inv.AddHost("h1", nil, "web")
	inv.AddHost("h2", nil, "db")
	hosts, err := inv.Match("web : db")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("Match(\"web : db\") = %v, want 2 hosts", hosts)
	}
}

func TestSplitPatternErrors(t *testing.T) {
	if _, err := splitPattern("web]1:5["); err == nil {
		t.Fatal("splitPattern with unbalanced ']': got nil error, want one")
	}
	if _, err := splitPattern("web[1:5"); err == nil {
		t.Fatal("splitPattern with unbalanced '[': got nil error, want one")
	}
}

func TestMatchTermRangeExpandsToGroupName(t *testing.T) {
	inv := New()
	inv.AddHost("h1", nil, "datacenter1")
	hosts, err := inv.Match("datacenter[1:1]")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Name != "h1" {
		t.Fatalf("Match(datacenter[1:1]) = %v, want [h1]", hosts)
	}
}

func TestMatchTermRangeExpandsToHostName(t *testing.T) {
	inv := New()
	inv.AddHost("web1", nil)
	hosts, err := inv.Match("web[1:1]")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Name != "web1" {
		t.Fatalf("Match(web[1:1]) = %v, want [web1]", hosts)
	}
}

func TestMatchTermGlobAgainstGroupName(t *testing.T) {
	inv := New()
	inv.AddHost("h1", nil, "datacenter1")
	hosts, err := inv.Match("datacenter*")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Name != "h1" {
		t.Fatalf("Match(datacenter*) = %v, want [h1] (glob matched via group name)", hosts)
	}
}

func TestExpandRangesEdgeCases(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{"stepped", "web[1:5:2]", []string{"web1", "web3", "web5"}, false},
		{"invalid step zero", "web[1:5:0]", nil, true},
		{"nonnumeric step doesn't match the range regex at all", "web[1:5:x]", []string{"web[1:5:x]"}, false},
		{"descending numeric", "web[5:1]", []string{"web5", "web4", "web3", "web2", "web1"}, false},
		{"descending alpha", "db[c:a]", []string{"dbc", "dbb", "dba"}, false},
		{"invalid mixed digit-run/single-letter", "x[12:c]y", nil, true},
		{"error in second range propagates from recursive expansion", "x[1:2]y[12:c]z", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := expandRanges(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expandRanges(%q): got nil error, want one", c.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("expandRanges(%q): %v", c.in, err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("expandRanges(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
