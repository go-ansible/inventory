package inventory

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func loadSampleYAML(t *testing.T) *Inventory {
	t.Helper()
	data, err := os.ReadFile("testdata/sample.yml")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := ParseYAML(data)
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	return inv
}

func TestParseYAMLStructure(t *testing.T) {
	inv := loadSampleYAML(t)

	wantHosts := []string{"web1.example.com", "web2.example.com", "db1.example.com"}
	for _, h := range wantHosts {
		if _, ok := inv.Hosts[h]; !ok {
			t.Errorf("missing host %q", h)
		}
	}

	for _, g := range []string{"all", "ungrouped", "webservers", "dbservers", "datacenter1"} {
		if _, ok := inv.Groups[g]; !ok {
			t.Errorf("missing group %q", g)
		}
	}

	if len(inv.Groups["ungrouped"].Hosts) != 0 {
		t.Errorf("ungrouped should be empty, got %v", inv.Groups["ungrouped"].Hosts)
	}

	dc := inv.Groups["datacenter1"]
	if _, ok := dc.Children["webservers"]; !ok {
		t.Error("datacenter1 should have webservers as a child")
	}
}

func TestHostVarsPrecedence(t *testing.T) {
	inv := loadSampleYAML(t)

	// web1: all.ntp_server, webservers.ansible_user=deploy, own http_port=80.
	v := inv.HostVars("web1.example.com")
	if v["ntp_server"] != "ntp.example.com" {
		t.Errorf("ntp_server = %v, want inherited from all", v["ntp_server"])
	}
	if v["ansible_user"] != "deploy" {
		t.Errorf("ansible_user = %v, want deploy (from webservers group)", v["ansible_user"])
	}
	if v["http_port"] != 80 {
		t.Errorf("http_port = %v, want 80", v["http_port"])
	}
	if v["region"] != "eu-west" {
		t.Errorf("region = %v, want eu-west (from datacenter1 group, via ancestry)", v["region"])
	}

	// web2 overrides ansible_user at host level: host vars win over group vars.
	v2 := inv.HostVars("web2.example.com")
	if v2["ansible_user"] != "web2admin" {
		t.Errorf("ansible_user = %v, want web2admin (host var overrides group var)", v2["ansible_user"])
	}
}

func TestMatchPatterns(t *testing.T) {
	inv := loadSampleYAML(t)

	cases := []struct {
		pattern string
		want    []string
	}{
		{"all", []string{"db1.example.com", "web1.example.com", "web2.example.com"}},
		{"webservers", []string{"web1.example.com", "web2.example.com"}},
		{"datacenter1", []string{"db1.example.com", "web1.example.com", "web2.example.com"}},
		{"web1.example.com", []string{"web1.example.com"}},
		{"webservers:dbservers", []string{"db1.example.com", "web1.example.com", "web2.example.com"}},
		{"webservers:!web1.example.com", []string{"web2.example.com"}},
		{"all:&webservers", []string{"web1.example.com", "web2.example.com"}},
		{"web*.example.com", []string{"web1.example.com", "web2.example.com"}},
	}
	for _, c := range cases {
		t.Run(c.pattern, func(t *testing.T) {
			hosts, err := inv.Match(c.pattern)
			if err != nil {
				t.Fatalf("Match(%q): %v", c.pattern, err)
			}
			var got []string
			for _, h := range hosts {
				got = append(got, h.Name)
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Match(%q) = %v, want %v", c.pattern, got, c.want)
			}
		})
	}
}

func TestExpandRanges(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"web[01:03].example.com", []string{"web01.example.com", "web02.example.com", "web03.example.com"}},
		{"web[1:3].example.com", []string{"web1.example.com", "web2.example.com", "web3.example.com"}},
		{"db[a:c]", []string{"dba", "dbb", "dbc"}},
		{"plain.example.com", []string{"plain.example.com"}},
	}
	for _, c := range cases {
		got, err := expandRanges(c.in)
		if err != nil {
			t.Fatalf("expandRanges(%q): %v", c.in, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("expandRanges(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseINIMatchesYAML(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.ini")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := ParseINI(data)
	if err != nil {
		t.Fatalf("ParseINI: %v", err)
	}

	if _, ok := inv.Hosts["web1.example.com"]; !ok {
		t.Fatal("web[1:2] range did not expand to web1.example.com")
	}
	v := inv.HostVars("web1.example.com")
	if v["ansible_user"] != "deploy" {
		t.Errorf("ansible_user = %v, want deploy", v["ansible_user"])
	}
	if v["http_port"] != 80 {
		t.Errorf("http_port = %v, want 80 (int)", v["http_port"])
	}
	if v["region"] != "eu-west" {
		t.Errorf("region = %v, want eu-west via datacenter1:children", v["region"])
	}
}

// --- Cross-validation against the reference ansible-inventory. ---

type ansibleInventoryOutput struct {
	Meta struct {
		Hostvars map[string]map[string]any `json:"hostvars"`
	} `json:"_meta"`
	Groups map[string]struct {
		Hosts    []string `json:"hosts"`
		Children []string `json:"children"`
	}
}

func ansibleInventoryBin(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("ANSIBLE_INVENTORY_BIN"); p != "" {
		return p
	}
	p, err := exec.LookPath("ansible-inventory")
	if err != nil {
		t.Skip("ansible-inventory not found in PATH; skipping cross-validation against the reference implementation")
	}
	return p
}

func TestInteropAgainstReferenceYAML(t *testing.T) {
	bin := ansibleInventoryBin(t)
	path, err := filepath.Abs("testdata/sample.yml")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "-i", path, "--list").Output()
	if err != nil {
		t.Fatalf("ansible-inventory: %v", err)
	}
	var ref ansibleInventoryOutput
	// ansible-inventory --list nests groups under top-level keys mixed
	// with "_meta"; decode generically then re-shape.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("decoding ansible-inventory output: %v\n%s", err, out)
	}
	if metaRaw, ok := raw["_meta"]; ok {
		if err := json.Unmarshal(metaRaw, &ref.Meta); err != nil {
			t.Fatal(err)
		}
		delete(raw, "_meta")
	}
	ref.Groups = map[string]struct {
		Hosts    []string `json:"hosts"`
		Children []string `json:"children"`
	}{}
	for name, body := range raw {
		var g struct {
			Hosts    []string `json:"hosts"`
			Children []string `json:"children"`
		}
		if err := json.Unmarshal(body, &g); err != nil {
			t.Fatal(err)
		}
		ref.Groups[name] = g
	}

	ours, err := ParseYAML(mustRead(t, "testdata/sample.yml"))
	if err != nil {
		t.Fatal(err)
	}

	for hostName, refVars := range ref.Meta.Hostvars {
		ourVars := ours.HostVars(hostName)
		for k, want := range refVars {
			got, ok := ourVars[k]
			if !ok {
				t.Errorf("host %s: missing var %q (reference has %v)", hostName, k, want)
				continue
			}
			if !equalJSONish(got, want) {
				t.Errorf("host %s var %q = %v (%T), reference = %v (%T)", hostName, k, got, got, want, want)
			}
		}
	}

	for groupName, refGroup := range ref.Groups {
		ourGroup, ok := ours.Groups[groupName]
		if !ok {
			t.Errorf("missing group %q (reference has it)", groupName)
			continue
		}
		wantHosts := append([]string{}, refGroup.Hosts...)
		sort.Strings(wantHosts)
		gotHosts := []string{}
		for name := range ourGroup.Hosts {
			gotHosts = append(gotHosts, name)
		}
		sort.Strings(gotHosts)
		if !reflect.DeepEqual(gotHosts, wantHosts) {
			t.Errorf("group %s hosts = %v, reference = %v", groupName, gotHosts, wantHosts)
		}
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// equalJSONish compares values the way two independently-decoded JSON
// documents should be compared: numeric types may differ (int vs
// float64) even when the value is the same.
func equalJSONish(a, b any) bool {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		return af == bf
	}
	return reflect.DeepEqual(a, b)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}
