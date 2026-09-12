package inventory

import (
	"os"
	"path/filepath"
	"strings"
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

// TestLoadWithVaultInlineScalar covers the shape ansible-vault
// encrypt_string produces: one !vault-tagged secret sitting in an
// otherwise-readable group_vars file, beside ordinary plaintext.
func TestLoadWithVaultInlineScalar(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "hosts.yml")
	if err := os.WriteFile(invPath, []byte("all:\n  hosts:\n    h1: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	enc, err := vault.Encrypt([]byte("s3cr3t"), "pw", "")
	if err != nil {
		t.Fatal(err)
	}
	var doc strings.Builder
	doc.WriteString("api_key: !vault |\n")
	for _, line := range strings.Split(strings.TrimRight(enc, "\n"), "\n") {
		doc.WriteString("          " + line + "\n")
	}
	doc.WriteString("region: eu-west\n")

	gv := filepath.Join(dir, "group_vars")
	if err := os.MkdirAll(gv, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gv, "all.yml"), []byte(doc.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	inv, err := LoadWithVault(invPath, "pw")
	if err != nil {
		t.Fatal(err)
	}
	g := inv.Groups["all"]
	if got := g.Vars["api_key"]; got != "s3cr3t" {
		t.Errorf("api_key = %#v, want the decrypted secret", got)
	}
	// The plaintext beside it is untouched.
	if got := g.Vars["region"]; got != "eu-west" {
		t.Errorf("region = %#v, want eu-west", got)
	}
}
