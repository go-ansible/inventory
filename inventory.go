// Package inventory parses Ansible-compatible inventories (YAML and INI),
// builds the group/host graph including group_vars/host_vars directories,
// and matches Ansible host patterns against it.
package inventory

import "sort"

// Host is one managed node: its name (as given in the inventory) plus the
// variables attached to it directly (not counting group-inherited vars —
// see Inventory.HostVars for the merged view).
type Host struct {
	Name string
	Vars map[string]any
}

// Group is a named collection of hosts and/or child groups, plus the
// variables attached to the group itself.
type Group struct {
	Name     string
	Hosts    map[string]*Host
	Children map[string]*Group
	Parents  map[string]*Group
	Vars     map[string]any
}

func newGroup(name string) *Group {
	return &Group{
		Name:     name,
		Hosts:    map[string]*Host{},
		Children: map[string]*Group{},
		Parents:  map[string]*Group{},
		Vars:     map[string]any{},
	}
}

// Inventory is the full host/group graph for one inventory source (a
// single file, or a directory merging several — see Load).
type Inventory struct {
	Hosts  map[string]*Host
	Groups map[string]*Group
}

// New returns an empty inventory pre-seeded with the two groups every
// Ansible inventory implicitly has: "all" (every host) and "ungrouped"
// (hosts in no other group).
func New() *Inventory {
	inv := &Inventory{
		Hosts:  map[string]*Host{},
		Groups: map[string]*Group{},
	}
	inv.group("all")
	inv.group("ungrouped")
	return inv
}

func (inv *Inventory) group(name string) *Group {
	g, ok := inv.Groups[name]
	if !ok {
		g = newGroup(name)
		inv.Groups[name] = g
	}
	return g
}

func (inv *Inventory) host(name string) *Host {
	h, ok := inv.Hosts[name]
	if !ok {
		h = &Host{Name: name, Vars: map[string]any{}}
		inv.Hosts[name] = h
	}
	return h
}

// addHostToGroup records host as a direct member of group. It does NOT
// also add the host to "all" directly — matching Ansible, whose "all"
// group only lists hosts assigned to it explicitly; every other host
// reaches "all" transitively through the group ancestry (see
// GroupsForHost), not through direct membership.
func (inv *Inventory) addHostToGroup(hostName, groupName string) *Host {
	h := inv.host(hostName)
	g := inv.group(groupName)
	g.Hosts[hostName] = h
	return h
}

func (inv *Inventory) addChild(parentName, childName string) {
	parent := inv.group(parentName)
	child := inv.group(childName)
	parent.Children[childName] = child
	child.Parents[parentName] = parent
}

// finalize computes "ungrouped": every host that belongs to no group
// other than "all".
func (inv *Inventory) finalize() {
	all := inv.group("all")
	ungrouped := inv.group("ungrouped")
	for name, h := range inv.Hosts {
		grouped := false
		for gname, g := range inv.Groups {
			if gname == "all" || gname == "ungrouped" {
				continue
			}
			if _, ok := g.Hosts[name]; ok {
				grouped = true
				break
			}
		}
		if !grouped {
			ungrouped.Hosts[name] = h
			all.Hosts[name] = h
		}
	}
}

// AddHost adds a host to the inventory at runtime — Ansible's add_host
// module — with the given variables, as a member of every named group
// (created if new). With no groups it is still reachable by the "all"
// pattern (matchTerm's "all"/"*" case iterates every known host
// directly) even though it joins no group's direct membership. Hosts
// already matched by an in-progress play are unaffected; only plays
// matched after this call see the addition, matching real Ansible.
func (inv *Inventory) AddHost(name string, vals map[string]any, groups ...string) {
	h := inv.host(name)
	for k, v := range vals {
		h.Vars[k] = v
	}
	for _, g := range groups {
		inv.addHostToGroup(name, g)
	}
	inv.finalize()
}

// AddToGroup records an existing (or new) host as a member of group,
// creating the group if it doesn't exist — Ansible's group_by module.
func (inv *Inventory) AddToGroup(hostName, groupName string) {
	inv.addHostToGroup(hostName, groupName)
	inv.finalize()
}

// ancestors returns g and every group that (transitively) contains g,
// ordered from the most distant ancestor to g itself — the order Ansible
// applies group vars in (parent group vars are overridden by child group
// vars).
func ancestors(g *Group) []*Group {
	seen := map[string]bool{}
	var order []*Group
	var visit func(*Group)
	visit = func(cur *Group) {
		var parents []*Group
		for _, p := range cur.Parents {
			parents = append(parents, p)
		}
		sort.Slice(parents, func(i, j int) bool { return parents[i].Name < parents[j].Name })
		for _, p := range parents {
			if !seen[p.Name] {
				seen[p.Name] = true
				visit(p)
				order = append(order, p)
			}
		}
	}
	visit(g)
	order = append(order, g)
	return order
}

// GroupsForHost returns every group host belongs to (directly or via a
// parent group), "all" first, deepest/most-specific last — the order
// group vars are merged in.
func (inv *Inventory) GroupsForHost(hostName string) []*Group {
	var direct []*Group
	names := make([]string, 0, len(inv.Groups))
	for name := range inv.Groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		g := inv.Groups[name]
		if _, ok := g.Hosts[hostName]; ok {
			direct = append(direct, g)
		}
	}

	seen := map[string]bool{}
	var out []*Group
	// "all" always comes first.
	for _, g := range direct {
		if g.Name == "all" {
			out = append(out, g)
			seen["all"] = true
		}
	}
	for _, g := range direct {
		for _, a := range ancestors(g) {
			if !seen[a.Name] {
				seen[a.Name] = true
				out = append(out, a)
			}
		}
	}
	return out
}

// HostVars returns the fully merged variables for hostName: group vars
// (from "all" down through parent groups to the most specific group,
// alphabetically among siblings) followed by the host's own vars, which
// win on conflict — matching Ansible's group-before-host precedence.
func (inv *Inventory) HostVars(hostName string) map[string]any {
	merged := map[string]any{}
	for _, g := range inv.GroupsForHost(hostName) {
		for k, v := range g.Vars {
			merged[k] = v
		}
	}
	if h, ok := inv.Hosts[hostName]; ok {
		for k, v := range h.Vars {
			merged[k] = v
		}
	}
	return merged
}

// Merge folds other into inv (later sources override earlier ones on
// scalar var conflicts, group/host membership is unioned) — how Ansible
// combines multiple inventory sources.
func (inv *Inventory) Merge(other *Inventory) {
	for name, h := range other.Hosts {
		dst := inv.host(name)
		for k, v := range h.Vars {
			dst.Vars[k] = v
		}
	}
	for name, g := range other.Groups {
		dst := inv.group(name)
		for k, v := range g.Vars {
			dst.Vars[k] = v
		}
		for hn := range g.Hosts {
			inv.addHostToGroup(hn, name)
		}
	}
	for name, g := range other.Groups {
		for cn := range g.Children {
			inv.addChild(name, cn)
		}
	}
	inv.finalize()
}
