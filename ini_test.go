package inventory

import (
	"strings"
	"testing"
)

func TestParseINIEdgeCases(t *testing.T) {
	t.Run("comments and blank lines", func(t *testing.T) {
		data := []byte("# a comment\n\n; another comment\nhost1\n")
		inv, err := ParseINI(data)
		if err != nil {
			t.Fatalf("ParseINI: %v", err)
		}
		if _, ok := inv.Hosts["host1"]; !ok {
			t.Fatal("host1 not parsed")
		}
	})

	t.Run("unterminated section header", func(t *testing.T) {
		_, err := ParseINI([]byte("[webservers\n"))
		if err == nil || !strings.Contains(err.Error(), "unterminated section header") {
			t.Fatalf("ParseINI: got %v, want unterminated section header error", err)
		}
	})

	t.Run("whitespace-only line is skipped", func(t *testing.T) {
		inv, err := ParseINI([]byte("[g]\n   \n"))
		if err != nil {
			t.Fatalf("ParseINI: %v", err)
		}
		if len(inv.Groups["g"].Hosts) != 0 {
			t.Fatalf("expected no hosts, got %v", inv.Groups["g"].Hosts)
		}
	})

	t.Run("children section with bad range", func(t *testing.T) {
		_, err := ParseINI([]byte("[dc:children]\nweb[12:c]\n"))
		if err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Fatalf("ParseINI: got %v, want a line-2 error", err)
		}
	})

	t.Run("vars section with bad kv", func(t *testing.T) {
		_, err := ParseINI([]byte("[g:vars]\nnotakeyvalue\n"))
		if err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Fatalf("ParseINI: got %v, want a line-2 error", err)
		}
	})

	t.Run("hosts section with bad range", func(t *testing.T) {
		_, err := ParseINI([]byte("[g]\nweb[12:c] foo=1\n"))
		if err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Fatalf("ParseINI: got %v, want a line-2 error", err)
		}
	})

	t.Run("hosts section with bad kv", func(t *testing.T) {
		_, err := ParseINI([]byte("[g]\nhost1 notakeyvalue\n"))
		if err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Fatalf("ParseINI: got %v, want a line-2 error", err)
		}
	})

	t.Run("quoted host var with embedded space", func(t *testing.T) {
		inv, err := ParseINI([]byte(`host1 description="edge node" tag='a b'` + "\n"))
		if err != nil {
			t.Fatalf("ParseINI: %v", err)
		}
		v := inv.HostVars("host1")
		if v["description"] != "edge node" {
			t.Errorf("description = %v, want %q", v["description"], "edge node")
		}
		if v["tag"] != "a b" {
			t.Errorf("tag = %v, want %q", v["tag"], "a b")
		}
	})

	t.Run("scanner error on an oversized line", func(t *testing.T) {
		// bufio.Scanner's default buffer caps a single token (line) at
		// bufio.MaxScanTokenSize (~64KB); a longer line with no newline
		// trips scanner.Err() with bufio.ErrTooLong.
		huge := strings.Repeat("x", 70*1024)
		_, err := ParseINI([]byte(huge))
		if err == nil {
			t.Fatal("ParseINI on an oversized line: got nil error, want one")
		}
	})

	t.Run("default section is ungrouped", func(t *testing.T) {
		inv, err := ParseINI([]byte("host1\n"))
		if err != nil {
			t.Fatalf("ParseINI: %v", err)
		}
		if _, ok := inv.Groups["ungrouped"].Hosts["host1"]; !ok {
			t.Fatal("host1 with no section should land in ungrouped")
		}
	})
}

func TestSplitHostLine(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{"simple", "host1 a=1 b=2", []string{"host1", "a=1", "b=2"}},
		{"double quoted value", `host1 desc="a b c"`, []string{"host1", `desc="a b c"`}},
		{"single quoted value", `host1 desc='a b c'`, []string{"host1", `desc='a b c'`}},
		{"tabs as separators", "host1\ta=1", []string{"host1", "a=1"}},
		{"leading/trailing spaces collapse", "  host1   a=1  ", []string{"host1", "a=1"}},
		{"empty", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitHostLine(c.line)
			if len(got) != len(c.want) {
				t.Fatalf("splitHostLine(%q) = %v, want %v", c.line, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("splitHostLine(%q) = %v, want %v", c.line, got, c.want)
				}
			}
		})
	}
}

func TestParseKV(t *testing.T) {
	t.Run("missing equals", func(t *testing.T) {
		if _, _, err := parseKV("noequals"); err == nil {
			t.Fatal("parseKV: got nil error, want one for missing '='")
		}
	})
	t.Run("double quoted", func(t *testing.T) {
		k, v, err := parseKV(`k="a b"`)
		if err != nil {
			t.Fatalf("parseKV: %v", err)
		}
		if k != "k" || v != "a b" {
			t.Fatalf("parseKV = %q, %v, want k, %q", k, v, "a b")
		}
	})
	t.Run("single quoted", func(t *testing.T) {
		k, v, err := parseKV(`k='a b'`)
		if err != nil {
			t.Fatalf("parseKV: %v", err)
		}
		if k != "k" || v != "a b" {
			t.Fatalf("parseKV = %q, %v, want k, %q", k, v, "a b")
		}
	})
	t.Run("unquoted coerced", func(t *testing.T) {
		k, v, err := parseKV("k=42")
		if err != nil {
			t.Fatalf("parseKV: %v", err)
		}
		if k != "k" || v != 42 {
			t.Fatalf("parseKV = %q, %v (%T), want k, 42", k, v, v)
		}
	})
}

func TestCoerce(t *testing.T) {
	cases := []struct {
		raw  string
		want any
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"false", false},
		{"False", false},
		{"FALSE", false},
		{"42", 42},
		{"-7", -7},
		{"3.14", 3.14},
		{"plainstring", "plainstring"},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			got := coerce(c.raw)
			if got != c.want {
				t.Fatalf("coerce(%q) = %v (%T), want %v (%T)", c.raw, got, got, c.want, c.want)
			}
		})
	}
}
