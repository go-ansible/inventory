package inventory

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Measured against ansible-core 2.21.4: each of these directories
// produces "Unable to parse <dir> as an inventory source" and the run
// continues on the implicit localhost. They are one case, not three --
// the filter that skips subdirectories, group_vars/host_vars and
// dotfiles leaves nothing behind in all of them.
func TestLoadDirectoryWithNoSources(t *testing.T) {
	cases := []struct {
		name  string
		build func(t *testing.T, dir string)
	}{{
		name:  "empty directory",
		build: func(t *testing.T, dir string) {},
	}, {
		name: "only group_vars",
		build: func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, "group_vars"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "group_vars", "all.yml"), []byte("x: 1\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	}, {
		name: "only a dotfile",
		build: func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("h1\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	}, {
		name: "only a subdirectory",
		build: func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.build(t, dir)
			inv, err := Load(dir)
			if !errors.Is(err, ErrNoSources) {
				t.Fatalf("Load(%s) err = %v, want ErrNoSources (inv=%v)", tc.name, err, inv)
			}
		})
	}
}

// A directory holding an inventory file still loads, even when that
// file is EMPTY: measured, real parses an empty .ini without
// complaint and only warns that the host list is empty.
func TestLoadDirectoryWithAnEmptyFileStillParses(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hosts.ini"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	inv, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v, want success: an empty FILE is a source, an empty DIRECTORY is not", err)
	}
	if len(inv.Hosts) != 0 {
		t.Fatalf("Hosts = %v, want none", inv.Hosts)
	}
}

// And one with a real file is untouched by any of this.
func TestLoadDirectoryWithASourceIsUnaffected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hosts.ini"), []byte("h1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := inv.Hosts["h1"]; !ok {
		t.Fatalf("Hosts = %v, want h1", inv.Hosts)
	}
}
