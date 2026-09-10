package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-ansible/vault"
)

// TestLoadWithVaultDecryptsGroupVars covers the canonical place real
// Ansible keeps secrets: an encrypted group_vars file next to the
// inventory. Load had no way to read one at all.
func TestLoadWithVaultDecryptsGroupVars(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) string {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	invPath := write("hosts.yml", "all:\n  hosts:\n    h1: {}\n")
	enc, err := vault.Encrypt([]byte("gv_secret: from_vault\n"), "pw", "")
	if err != nil {
		t.Fatal(err)
	}
	write("group_vars/all.yml", enc)

	// Without a password the encrypted file is a clear error, not a
	// YAML parse failure on ciphertext.
	if _, err := Load(invPath); err == nil {
		t.Error("encrypted group_vars with no password: got nil error, want one")
	}

	inv, err := LoadWithVault(invPath, "pw")
	if err != nil {
		t.Fatal(err)
	}
	g, ok := inv.Groups["all"]
	if !ok {
		t.Fatal("group all missing")
	}
	if got := g.Vars["gv_secret"]; got != "from_vault" {
		t.Errorf("gv_secret = %#v, want from_vault", got)
	}
}

// TestLoadWithVaultReadsAnEncryptedInventory covers the inventory file
// itself being encrypted, which real Ansible also accepts.
func TestLoadWithVaultReadsAnEncryptedInventory(t *testing.T) {
	dir := t.TempDir()
	enc, err := vault.Encrypt([]byte("all:\n  hosts:\n    secret-host: {}\n"), "pw", "")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "hosts.yml")
	if err := os.WriteFile(p, []byte(enc), 0o600); err != nil {
		t.Fatal(err)
	}

	inv, err := LoadWithVault(p, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := inv.Hosts["secret-host"]; !ok {
		t.Errorf("hosts = %v, want secret-host", inv.Hosts)
	}
}

// TestLoadStillWorksWithoutVault guards that plaintext loading is
// unchanged — Load is LoadWithVault with an empty password.
func TestLoadStillWorksWithoutVault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hosts.yml")
	if err := os.WriteFile(p, []byte("all:\n  hosts:\n    h1: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inv, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := inv.Hosts["h1"]; !ok {
		t.Errorf("hosts = %v, want h1", inv.Hosts)
	}
}
