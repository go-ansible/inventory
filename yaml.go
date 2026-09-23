package inventory

import (
	"fmt"
	"github.com/go-ansible/vault"

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
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("inventory: parsing YAML: %w", err)
	}
	// The same implicit scalar resolution UnmarshalYAML does for
	// everything else in this port: yes/no/on/off become booleans and
	// base-60 scalars numbers, as they do in real Ansible. Applied to
	// the tree because this parser walks nodes itself rather than
	// decoding into a map — which is what preserves host order.
	vault.ResolveYAML11(&doc)
	root := mappingOf(&doc)
	if root == nil {
		return New(), nil // an empty document has no groups
	}
	inv := New()
	for i := 0; i+1 < len(root.Content); i += 2 {
		if err := inv.parseYAMLGroup(root.Content[i].Value, root.Content[i+1]); err != nil {
			return nil, err
		}
	}
	inv.finalize()
	return inv, nil
}

// mappingOf unwraps a document node and returns it if it is a mapping,
// nil otherwise. Walking NODES rather than a decoded map[string]any is
// what preserves the order hosts and groups were written in — which is
// the order real lists and runs them in, and which a Go map destroys.
func mappingOf(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil
		}
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	return n
}

// entry returns the value node for key in a mapping, or nil.
func entry(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func (inv *Inventory) parseYAMLGroup(name string, body *yaml.Node) error {
	inv.group(name)

	if body == nil || body.Tag == "!!null" {
		return nil // e.g. `group1:` with no body
	}
	m := mappingOf(body)
	if m == nil {
		return fmt.Errorf("inventory: group %q: expected a mapping, got %s", name, body.Tag)
	}

	if hosts := entry(m, "hosts"); hosts != nil && hosts.Tag != "!!null" {
		hm := mappingOf(hosts)
		if hm == nil {
			return fmt.Errorf("inventory: group %q: hosts must be a mapping", name)
		}
		for i := 0; i+1 < len(hm.Content); i += 2 {
			h := inv.addHostToGroup(hm.Content[i].Value, name)
			var hostVars map[string]any
			if err := hm.Content[i+1].Decode(&hostVars); err == nil {
				for k, v := range hostVars {
					h.Vars[k] = v
				}
			}
		}
	}

	if varsNode := entry(m, "vars"); varsNode != nil && varsNode.Tag != "!!null" {
		var groupVars map[string]any
		if err := varsNode.Decode(&groupVars); err != nil {
			return fmt.Errorf("inventory: group %q: vars must be a mapping", name)
		}
		g := inv.group(name)
		for k, v := range groupVars {
			g.Vars[k] = v
		}
	}

	if children := entry(m, "children"); children != nil && children.Tag != "!!null" {
		cm := mappingOf(children)
		if cm == nil {
			return fmt.Errorf("inventory: group %q: children must be a mapping", name)
		}
		for i := 0; i+1 < len(cm.Content); i += 2 {
			childName := cm.Content[i].Value
			inv.addChild(name, childName)
			if err := inv.parseYAMLGroup(childName, cm.Content[i+1]); err != nil {
				return err
			}
		}
	}

	return nil
}
