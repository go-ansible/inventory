package inventory

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// IsScript reports whether a file with this mode should be treated as
// a dynamic inventory script rather than a static INI/YAML file — real
// Ansible's own detection: any executable bit set on the file (not a
// directory), regardless of extension.
func IsScript(mode os.FileMode) bool {
	return mode.Perm()&0o111 != 0
}

// scriptGroup is one entry of a dynamic inventory script's --list
// output — either the shorthand form (a bare JSON array of hostnames)
// or the full form ({"hosts": [...], "vars": {...}, "children": [...]}).
// UnmarshalJSON below accepts both, matching real Ansible's own script
// inventory contract.
type scriptGroup struct {
	Hosts    []string       `json:"hosts"`
	Vars     map[string]any `json:"vars"`
	Children []string       `json:"children"`
}

func (g *scriptGroup) UnmarshalJSON(data []byte) error {
	var hosts []string
	if err := json.Unmarshal(data, &hosts); err == nil {
		g.Hosts = hosts
		return nil
	}
	type alias scriptGroup
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*g = scriptGroup(a)
	return nil
}

// ParseScript runs the dynamic inventory script at path — real
// Ansible's inventory-script contract: `path --list` prints one JSON
// document to stdout describing every group (either the shorthand
// `["host1","host2"]` array form or the full `{"hosts":[...],
// "vars":{...},"children":[...]}` form) plus an optional top-level
// "_meta" key holding `{"hostvars": {"host1": {...}, ...}}`.
func ParseScript(path string) (*Inventory, error) {
	listData, err := runScript(path, "--list")
	if err != nil {
		return nil, fmt.Errorf("inventory: script %s --list: %w", path, err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(listData, &raw); err != nil {
		return nil, fmt.Errorf("inventory: script %s --list: invalid JSON: %w", path, err)
	}

	var meta struct {
		HostVars map[string]map[string]any `json:"hostvars"`
	}
	if metaRaw, ok := raw["_meta"]; ok {
		if err := json.Unmarshal(metaRaw, &meta); err != nil {
			return nil, fmt.Errorf("inventory: script %s --list: invalid _meta: %w", path, err)
		}
		delete(raw, "_meta")
	}

	inv := New()
	for name, groupRaw := range raw {
		var g scriptGroup
		if err := json.Unmarshal(groupRaw, &g); err != nil {
			return nil, fmt.Errorf("inventory: script %s --list: group %q: %w", path, name, err)
		}
		dst := inv.group(name)
		for k, v := range g.Vars {
			dst.Vars[k] = v
		}
		for _, h := range g.Hosts {
			inv.addHostToGroup(h, name)
		}
		for _, c := range g.Children {
			inv.addChild(name, c)
		}
	}

	// _meta.hostvars is the recommended, efficient form (real Ansible's
	// own docs: avoids a --host round trip per host) and is what most
	// real dynamic inventory scripts (cloud-provider ones especially)
	// emit. A script that omits it entirely is supported too, via the
	// --host fallback below, matching real Ansible's own behavior in
	// that case — one process spawn per discovered host.
	if meta.HostVars != nil {
		for name, vars := range meta.HostVars {
			h := inv.host(name)
			for k, v := range vars {
				h.Vars[k] = v
			}
		}
	} else {
		for name := range inv.Hosts {
			hostData, err := runScript(path, "--host", name)
			if err != nil {
				return nil, fmt.Errorf("inventory: script %s --host %s: %w", path, name, err)
			}
			var vars map[string]any
			if err := json.Unmarshal(hostData, &vars); err != nil {
				return nil, fmt.Errorf("inventory: script %s --host %s: invalid JSON: %w", path, name, err)
			}
			h := inv.host(name)
			for k, v := range vars {
				h.Vars[k] = v
			}
		}
	}

	inv.finalize()
	return inv, nil
}

func runScript(path string, args ...string) ([]byte, error) {
	cmd := exec.Command(path, args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("exit %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, err
	}
	return out, nil
}
