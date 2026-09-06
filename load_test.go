package inventory

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSingleYAMLFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.yml")
	writeFile(t, path, "all:\n  hosts:\n    h1: {}\n")

	inv, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := inv.Hosts["h1"]; !ok {
		t.Fatal("h1 not loaded")
	}
}

func TestLoadSingleINIFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts") // no .yml/.yaml extension -> INI
	writeFile(t, path, "[web]\nh1 a=1\n")

	inv, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	h, ok := inv.Hosts["h1"]
	if !ok {
		t.Fatal("h1 not loaded")
	}
	if h.Vars["a"] != 1 {
		t.Fatalf("h1.a = %v, want 1", h.Vars["a"])
	}
}

func TestLoadFileExtensionCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.YML")
	writeFile(t, path, "all:\n  hosts:\n    h1: {}\n")

	inv, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := inv.Hosts["h1"]; !ok {
		t.Fatal("h1 not loaded via uppercase .YML extension")
	}
}

func TestLoadDirectoryMergesInNameOrder(t *testing.T) {
	dir := t.TempDir()
	// "a" sets x=1, "b" overrides x=2: name order means b applies after a.
	writeFile(t, filepath.Join(dir, "a.yml"), "all:\n  vars:\n    x: 1\n  hosts:\n    h1: {}\n")
	writeFile(t, filepath.Join(dir, "b.yml"), "all:\n  vars:\n    x: 2\n  hosts:\n    h2: {}\n")
	// A dotfile and a subdirectory should both be skipped from the merge.
	writeFile(t, filepath.Join(dir, ".hidden.yml"), "all:\n  vars:\n    x: 99\n")
	writeFile(t, filepath.Join(dir, "subdir", "c.yml"), "all:\n  vars:\n    x: 99\n")

	inv, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if inv.Groups["all"].Vars["x"] != 2 {
		t.Fatalf("all.x = %v, want 2 (b.yml applied after a.yml, dotfile/subdir skipped)", inv.Groups["all"].Vars["x"])
	}
	for _, want := range []string{"h1", "h2"} {
		if _, ok := inv.Hosts[want]; !ok {
			t.Errorf("missing host %q", want)
		}
	}
}

func TestLoadGroupVarsFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hosts.yml"), "all:\n  children:\n    web:\n      hosts:\n        h1: {}\n")
	writeFile(t, filepath.Join(dir, "group_vars", "web.yml"), "port: 80\n")

	inv, err := Load(filepath.Join(dir, "hosts.yml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if inv.Groups["web"].Vars["port"] != 80 {
		t.Fatalf("web.port = %v, want 80", inv.Groups["web"].Vars["port"])
	}
}

func TestLoadGroupVarsDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hosts.yml"), "all:\n  children:\n    web:\n      hosts:\n        h1: {}\n")
	writeFile(t, filepath.Join(dir, "group_vars", "web", "a.yml"), "port: 80\n")
	writeFile(t, filepath.Join(dir, "group_vars", "web", "b.yml"), "port: 8080\n")

	inv, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if inv.Groups["web"].Vars["port"] != 8080 {
		t.Fatalf("web.port = %v, want 8080 (b.yml applied after a.yml)", inv.Groups["web"].Vars["port"])
	}
}

func TestLoadHostVarsFileAndDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hosts.yml"), "all:\n  hosts:\n    h1: {}\n")
	writeFile(t, filepath.Join(dir, "host_vars", "h1.yml"), "a: 1\n")

	inv, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if inv.Hosts["h1"].Vars["a"] != 1 {
		t.Fatalf("h1.a = %v, want 1", inv.Hosts["h1"].Vars["a"])
	}
}

func TestLoadVarsFilePrecedesOverDirectorySameName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hosts.yml"), "all:\n  children:\n    prod:\n      hosts:\n        h1: {}\n")
	writeFile(t, filepath.Join(dir, "group_vars", "prod", "a.yml"), "region: dir\n")
	writeFile(t, filepath.Join(dir, "group_vars", "prod.yml"), "region: file\n")

	inv, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Directory entries sort before "<name>.yml" (it is a strict prefix),
	// so the file is merged last and wins.
	if inv.Groups["prod"].Vars["region"] != "file" {
		t.Fatalf("prod.region = %v, want %q (file overrides directory of same name)", inv.Groups["prod"].Vars["region"], "file")
	}
}

func TestLoadNonexistentPath(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("Load on nonexistent path: got nil error, want one")
	}
}

func TestLoadDirectoryWithBadFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad.yml"), "all: [unterminated\n")
	if _, err := Load(dir); err == nil {
		t.Fatal("Load with malformed file in directory: got nil error, want one")
	}
}

func TestLoadGroupVarsNotADirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hosts.yml"), "all:\n  hosts:\n    h1: {}\n")
	// group_vars exists but is a regular file, not a directory: os.ReadDir
	// fails with something other than IsNotExist.
	writeFile(t, filepath.Join(dir, "group_vars"), "not a directory\n")

	if _, err := Load(dir); err == nil {
		t.Fatal("Load with group_vars as a regular file: got nil error, want one")
	}
}

func TestLoadGroupVarsMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hosts.yml"), "all:\n  children:\n    web:\n      hosts:\n        h1: {}\n")
	writeFile(t, filepath.Join(dir, "group_vars", "web.yml"), "bad: [unterminated\n")

	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Fatalf("Load with malformed group_vars file: got %v, want a parsing error", err)
	}
}

func TestLoadHostVarsNotADirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hosts.yml"), "all:\n  hosts:\n    h1: {}\n")
	// host_vars exists but is a regular file, not a directory.
	writeFile(t, filepath.Join(dir, "host_vars"), "not a directory\n")

	if _, err := Load(dir); err == nil {
		t.Fatal("Load with host_vars as a regular file: got nil error, want one")
	}
}

func TestLoadGroupVarsNestedDirectoryMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hosts.yml"), "all:\n  children:\n    web:\n      hosts:\n        h1: {}\n")
	writeFile(t, filepath.Join(dir, "group_vars", "web", "a.yml"), "bad: [unterminated\n")

	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Fatalf("Load with malformed group_vars/web/a.yml: got %v, want a parsing error", err)
	}
}

func skipUnlessPermsEnforced(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
}

func TestLoadDirectoryUnreadable(t *testing.T) {
	skipUnlessPermsEnforced(t)
	parent := t.TempDir()
	dir := filepath.Join(parent, "inv")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "hosts.yml"), "all: {}\n")
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)

	if _, err := Load(dir); err == nil {
		t.Fatal("Load on an unreadable directory: got nil error, want one")
	}
}

func TestLoadGroupVarsInnerDirectoryUnreadable(t *testing.T) {
	skipUnlessPermsEnforced(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hosts.yml"), "all:\n  children:\n    web:\n      hosts:\n        h1: {}\n")
	inner := filepath.Join(dir, "group_vars", "web")
	writeFile(t, filepath.Join(inner, "a.yml"), "port: 80\n")
	if err := os.Chmod(inner, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(inner, 0o755)

	if _, err := Load(dir); err == nil {
		t.Fatal("Load with an unreadable group_vars/web/ directory: got nil error, want one")
	}
}

func TestLoadGroupVarsFileUnreadable(t *testing.T) {
	skipUnlessPermsEnforced(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hosts.yml"), "all:\n  children:\n    web:\n      hosts:\n        h1: {}\n")
	varsFile := filepath.Join(dir, "group_vars", "web.yml")
	writeFile(t, varsFile, "port: 80\n")
	if err := os.Chmod(varsFile, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(varsFile, 0o644)

	if _, err := Load(dir); err == nil {
		t.Fatal("Load with an unreadable group_vars/web.yml: got nil error, want one")
	}
}

func TestLoadUnreadableFile(t *testing.T) {
	skipUnlessPermsEnforced(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.yml")
	writeFile(t, path, "all: {}\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o644)

	if _, err := Load(path); err == nil {
		t.Fatal("Load on an unreadable file: got nil error, want one")
	}
}

func writeScript(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestLoadDynamicInventoryScriptWithMeta locks in real Ansible's
// dynamic-inventory-script contract (executable file, --list prints
// JSON, "_meta.hostvars" is the efficient form) with a fixture
// verified byte-for-byte against a real `ansible-inventory --list`
// run: web1/web2 (full-form group "web", with a group var) and db1
// (shorthand array-form group "db") both inherit env=production
// through "prod" listing them as children — real Ansible's own
// hostvars for web1 there is exactly
// {ansible_connection, env, ip, role}, matching this test's want.
func TestLoadDynamicInventoryScriptWithMeta(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inv.sh")
	writeScript(t, path, `#!/bin/sh
cat <<'JSON'
{
  "web": {"hosts": ["web1", "web2"], "vars": {"role": "webserver"}},
  "db": ["db1"],
  "prod": {"children": ["web", "db"], "vars": {"env": "production"}},
  "_meta": {"hostvars": {
    "web1": {"ansible_connection": "local", "ip": "10.0.0.1"},
    "web2": {"ansible_connection": "local", "ip": "10.0.0.2"},
    "db1": {"ansible_connection": "local", "ip": "10.0.0.3"}
  }}
}
JSON
`)
	inv, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]map[string]any{
		"web1": {"ansible_connection": "local", "env": "production", "ip": "10.0.0.1", "role": "webserver"},
		"web2": {"ansible_connection": "local", "env": "production", "ip": "10.0.0.2", "role": "webserver"},
		"db1":  {"ansible_connection": "local", "env": "production", "ip": "10.0.0.3"},
	}
	for host, wantVars := range want {
		got := inv.HostVars(host)
		if len(got) != len(wantVars) {
			t.Errorf("%s: HostVars = %v, want %v", host, got, wantVars)
			continue
		}
		for k, v := range wantVars {
			if got[k] != v {
				t.Errorf("%s: HostVars[%q] = %v, want %v", host, k, got[k], v)
			}
		}
	}
}

// TestLoadDynamicInventoryScriptHostFallback locks in the --host
// fallback for a script that omits _meta entirely — real Ansible's own
// documented behavior in that case, verified against a real
// ansible-inventory run of an equivalent fixture.
func TestLoadDynamicInventoryScriptHostFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inv.sh")
	writeScript(t, path, `#!/bin/sh
if [ "$1" = "--host" ]; then
  case "$2" in
    web1) echo '{"ip": "10.0.0.1"}' ;;
    web2) echo '{"ip": "10.0.0.2"}' ;;
    *) echo '{}' ;;
  esac
else
  echo '{"web": {"hosts": ["web1", "web2"]}}'
fi
`)
	inv, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := inv.HostVars("web1")["ip"]; got != "10.0.0.1" {
		t.Errorf("web1 ip = %v, want 10.0.0.1", got)
	}
	if got := inv.HostVars("web2")["ip"]; got != "10.0.0.2" {
		t.Errorf("web2 ip = %v, want 10.0.0.2", got)
	}
}

func TestLoadDynamicInventoryScriptFailureIsAnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inv.sh")
	writeScript(t, path, "#!/bin/sh\necho 'not json' >&2\nexit 1\n")
	if _, err := Load(path); err == nil {
		t.Fatal("Load on a failing inventory script: got nil error, want one")
	}
}
