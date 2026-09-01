package inventory

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParseYAML parses an Ansible YAML inventory (the format produced by
// `ansible-inventory --list` / documented as "YAML inventory"):
//
//	all:
//	  hosts:
//	    host1: {}
//	  vars:
//	    v: 1
//	  children:
//	    group1:
//	      hosts:
//	        host2:
//	          v: 2
func ParseYAML(data []byte) (*Inventory, error) {
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("inventory: parsing YAML: %w", err)
	}
	inv := New()
	for name, body := range root {
		if err := inv.parseYAMLGroup(name, body); err != nil {
			return nil, err
		}
	}
	inv.finalize()
	return inv, nil
}

func (inv *Inventory) parseYAMLGroup(name string, body any) error {
	inv.group(name)

	m, ok := body.(map[string]any)
	if !ok {
		if body == nil {
			return nil // e.g. `group1:` with no body
		}
		return fmt.Errorf("inventory: group %q: expected a mapping, got %T", name, body)
	}

	if hostsRaw, ok := m["hosts"]; ok {
		hosts, ok := hostsRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("inventory: group %q: hosts must be a mapping", name)
		}
		for hostName, hostBody := range hosts {
			h := inv.addHostToGroup(hostName, name)
			if hm, ok := hostBody.(map[string]any); ok {
				for k, v := range hm {
					h.Vars[k] = v
				}
			}
		}
	}

	if varsRaw, ok := m["vars"]; ok {
		vars, ok := varsRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("inventory: group %q: vars must be a mapping", name)
		}
		g := inv.group(name)
		for k, v := range vars {
			g.Vars[k] = v
		}
	}

	if childrenRaw, ok := m["children"]; ok {
		children, ok := childrenRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("inventory: group %q: children must be a mapping", name)
		}
		for childName, childBody := range children {
			inv.addChild(name, childName)
			if err := inv.parseYAMLGroup(childName, childBody); err != nil {
				return err
			}
		}
	}

	return nil
}
