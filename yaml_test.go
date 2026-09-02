package inventory

import (
	"strings"
	"testing"
)

func TestParseYAMLErrors(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{
			name: "malformed yaml",
			data: "all: [unterminated\n",
			want: "parsing YAML",
		},
		{
			name: "group body not a mapping",
			data: "all: notamapping\n",
			want: `expected a mapping`,
		},
		{
			name: "hosts not a mapping",
			data: "all:\n  hosts: notamapping\n",
			want: "hosts must be a mapping",
		},
		{
			name: "vars not a mapping",
			data: "all:\n  vars: notamapping\n",
			want: "vars must be a mapping",
		},
		{
			name: "children not a mapping",
			data: "all:\n  children: notamapping\n",
			want: "children must be a mapping",
		},
		{
			name: "error in nested child group propagates",
			data: "all:\n  children:\n    g1:\n      vars: notamapping\n",
			want: "vars must be a mapping",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseYAML([]byte(c.data))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("ParseYAML(%q): got %v, want error containing %q", c.data, err, c.want)
			}
		})
	}
}

func TestParseYAMLGroupWithNilBody(t *testing.T) {
	inv, err := ParseYAML([]byte("all:\nemptygroup:\n"))
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	if _, ok := inv.Groups["emptygroup"]; !ok {
		t.Fatal("emptygroup with no body should still be created")
	}
}

func TestParseYAMLHostWithScalarBody(t *testing.T) {
	// A host with a non-mapping body (e.g. null) should be created with
	// no extra vars, not error.
	inv, err := ParseYAML([]byte("all:\n  hosts:\n    host1: null\n"))
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	h, ok := inv.Hosts["host1"]
	if !ok {
		t.Fatal("host1 not created")
	}
	if len(h.Vars) != 0 {
		t.Fatalf("host1 vars = %v, want empty", h.Vars)
	}
}
