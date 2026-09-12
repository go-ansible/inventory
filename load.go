package inventory

import (
	"fmt"
	"github.com/go-ansible/vault"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Load reads an inventory from path, which may be:
//   - a single YAML file (.yml/.yaml)
//   - a single INI file (any other extension, or none)
//   - a directory, in which every regular file directly inside it (except
//     group_vars/ and host_vars/) is parsed and merged, in name order
//
// group_vars/<name>.yml, group_vars/<name>/*.yml, host_vars/<name>.yml
// and host_vars/<name>/*.yml siblings of the source are then merged in,
// group vars before host vars, per Ansible's precedence.
func Load(path string) (*Inventory, error) {
	return LoadWithVault(path, "")
}

// LoadWithVault is Load with a vault password, so an inventory file or
// any group_vars/host_vars file it merges may be vault-encrypted — the
// canonical place real Ansible keeps secrets is a group_vars/*/vault.yml.
// An empty password behaves exactly like Load: a plaintext inventory
// loads fine, and an encrypted one is a clear error rather than a YAML
// parse failure on ciphertext.
func LoadWithVault(path, vaultPassword string) (*Inventory, error) {
	return loadWith(path, vaultPassword)
}

func loadWith(path, vaultPassword string) (*Inventory, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inventory: %w", err)
	}

	inv := New()
	var baseDirs []string

	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("inventory: %w", err)
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() || e.Name() == "group_vars" || e.Name() == "host_vars" {
				continue
			}
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			sub, err := loadFile(filepath.Join(path, name), vaultPassword)
			if err != nil {
				return nil, err
			}
			inv.Merge(sub)
		}
		baseDirs = append(baseDirs, path)
	} else if IsScript(info.Mode()) {
		// A dynamic inventory script's own output is the complete
		// inventory — no group_vars/host_vars directory merge applies
		// (real Ansible does not look for one next to a script either),
		// so return directly rather than falling through to the
		// baseDirs handling below.
		return ParseScript(path)
	} else {
		sub, err := loadFile(path, vaultPassword)
		if err != nil {
			return nil, err
		}
		inv.Merge(sub)
		baseDirs = append(baseDirs, filepath.Dir(path))
	}

	for _, dir := range baseDirs {
		if err := mergeVarsDir(inv, filepath.Join(dir, "group_vars"), groupVarsTarget(inv), vaultPassword); err != nil {
			return nil, err
		}
		if err := mergeVarsDir(inv, filepath.Join(dir, "host_vars"), hostVarsTarget(inv), vaultPassword); err != nil {
			return nil, err
		}
	}

	return inv, nil
}

func loadFile(path, vaultPassword string) (*Inventory, error) {
	data, err := readMaybeEncrypted(path, vaultPassword)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".yml" || ext == ".yaml" {
		return ParseYAML(data)
	}
	return ParseINI(data)
}

func groupVarsTarget(inv *Inventory) func(name string) map[string]any {
	return func(name string) map[string]any {
		return inv.group(name).Vars
	}
}

func hostVarsTarget(inv *Inventory) func(name string) map[string]any {
	return func(name string) map[string]any {
		return inv.host(name).Vars
	}
}

// mergeVarsDir loads group_vars/ or host_vars/, applying each <name>.yml
// or <name>/*.yml (merged in name order) into targetFor(name).
func mergeVarsDir(inv *Inventory, dir string, targetFor func(name string) map[string]any, vaultPassword string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inventory: %w", err)
	}
	for _, e := range entries {
		full := filepath.Join(dir, e.Name())
		if e.IsDir() {
			inner, err := os.ReadDir(full)
			if err != nil {
				return fmt.Errorf("inventory: %w", err)
			}
			var names []string
			for _, f := range inner {
				if !f.IsDir() {
					names = append(names, f.Name())
				}
			}
			sort.Strings(names)
			target := targetFor(e.Name())
			for _, name := range names {
				if err := mergeVarsFile(filepath.Join(full, name), target, vaultPassword); err != nil {
					return err
				}
			}
			continue
		}
		name := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".yml"), ".yaml")
		if err := mergeVarsFile(full, targetFor(name), vaultPassword); err != nil {
			return err
		}
	}
	return nil
}

func mergeVarsFile(path string, target map[string]any, vaultPassword string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("inventory: %w", err)
	}
	// vault.UnmarshalYAML handles both shapes a secret can take here: the
	// whole file encrypted, or individual !vault-tagged scalars sitting
	// in an otherwise-readable group_vars file, which is how one secret
	// lives beside plaintext.
	var vars map[string]any
	if err := vault.UnmarshalYAML(data, vaultPassword, &vars); err != nil {
		return fmt.Errorf("inventory: parsing %s: %w", path, err)
	}
	for k, v := range vars {
		target[k] = v
	}
	return nil
}

// readMaybeEncrypted reads a file and decrypts it when it turns out to be
// vault-encrypted, so an inventory or a group_vars/host_vars file can be
// either without the call site knowing which.
func readMaybeEncrypted(path, vaultPassword string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("inventory: %w", err)
	}
	plain, err := vault.MaybeDecrypt(data, vaultPassword)
	if err != nil {
		return nil, fmt.Errorf("inventory: %s: %w", path, err)
	}
	return plain, nil
}
