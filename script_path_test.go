package inventory

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A dynamic inventory script is chosen by stat'ing a file and reading its
// mode, and then it is EXECUTED. Those must be the same file.
//
// exec.Command does a $PATH lookup for any name with no separator in it,
// so `-i inv.sh` used to stat ./inv.sh, conclude from that file's mode
// that it was a script, and then run a different inv.sh found somewhere
// on $PATH. Measured against real ansible-core 2.21.4 with the same
// setup: real runs the file it was pointed at, and names it absolutely
// in its own diagnostics.

func writeInventoryScript(t *testing.T, path, host string) {
	t.Helper()
	body := "#!/bin/sh\nif [ \"$1\" = --list ]; then printf '{\"all\":{\"hosts\":[\"" + host + "\"]}}'; else printf '{}'; fi\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

const (
	namedHost    = "FROM-THE-FILE-WE-NAMED"
	impostorHost = "FROM-SOMEWHERE-ON-PATH"
)

func onlyHost(t *testing.T, inv *Inventory) string {
	t.Helper()
	for _, h := range []string{namedHost, impostorHost} {
		if _, ok := inv.Hosts[h]; ok {
			return h
		}
	}
	t.Fatalf("inventory held neither witness host; got %d hosts", len(inv.Hosts))
	return ""
}

// armed sets up a working directory holding the script we will name, and
// an impostor of the SAME basename earlier on $PATH. It returns nothing:
// the control below is what proves the trap is actually armed.
func armed(t *testing.T) {
	t.Helper()
	work, impostor := t.TempDir(), t.TempDir()
	writeInventoryScript(t, filepath.Join(work, "inv.sh"), namedHost)
	writeInventoryScript(t, filepath.Join(impostor, "inv.sh"), impostorHost)
	t.Setenv("PATH", impostor+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Chdir(work)
}

// CONTROL: a bare "inv.sh" really does resolve to the impostor through a
// $PATH lookup. Without this, the test below could pass because the
// impostor was never reachable, which would prove nothing at all.
func TestTheImpostorIsReachableOnPath(t *testing.T) {
	armed(t)
	resolved, err := exec.LookPath("inv.sh")
	if err != nil {
		t.Fatalf("trap is NOT armed: a bare name does not resolve on $PATH at all: %v", err)
	}
	out, err := exec.Command(resolved, "--list").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); !strings.Contains(got, impostorHost) {
		t.Fatalf("trap is NOT armed: a bare-name lookup ran %q, whose output was %s", resolved, got)
	}
}

func TestAScriptRunsTheFileWeWerePointedAt(t *testing.T) {
	armed(t)
	inv, err := Load("inv.sh")
	if err != nil {
		t.Fatal(err)
	}
	if got := onlyHost(t, inv); got != namedHost {
		t.Errorf("Load(%q) ran the wrong file: host %q, want %q — a $PATH lookup beat the file we stat'd",
			"inv.sh", got, namedHost)
	}
}

func TestAnExplicitRelativePathStillWorks(t *testing.T) {
	armed(t)
	inv, err := Load("./inv.sh")
	if err != nil {
		t.Fatal(err)
	}
	if got := onlyHost(t, inv); got != namedHost {
		t.Errorf("Load(\"./inv.sh\") ran the wrong file: host %q, want %q", got, namedHost)
	}
}
