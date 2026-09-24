package inventory

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Match resolves an Ansible host pattern ("all", a group name, a glob, a
// numeric/alpha range, or a colon/comma-separated combination with `!`
// exclusion and `&` intersection, e.g. "webservers:!web3:&datacenter1")
// against the inventory, and returns the matching hosts in inventory
// order.
func (inv *Inventory) Match(pattern string) ([]*Host, error) {
	hosts, _, err := inv.MatchReport(pattern)
	return hosts, err
}

// MatchReport is Match, and additionally reports each TERM of the
// pattern that matched neither a host nor a group. Real Ansible warns
// about exactly those, one line per term — "Could not match supplied
// host pattern, ignoring: <term>" — so that a mistyped pattern is
// distinguishable from one that legitimately selects nothing, and a
// caller that reports to a user wants them.
//
// Terms are reported WITHOUT their `!`/`&` operator and in real's own
// evaluation order (see orderPatterns), because that is the spelling
// and the order real's own warnings use: `all:!zzz` reports `zzz`, and
// `aaa,bbb` reports `aaa` then `bbb`. A term naming an EMPTY GROUP is
// not reported, nor is `all` or `*` — see matchTerm for why those are
// never mistyped patterns.
func (inv *Inventory) MatchReport(pattern string) ([]*Host, []string, error) {
	// An IPv6 literal is all colons, and the pattern language uses a
	// colon as its separator — so "::1" must be recognised BEFORE the
	// split, or it becomes three empty terms.
	if h := inv.implicitLocalhost(strings.TrimSpace(pattern)); h != nil {
		return []*Host{h}, nil, nil
	}
	terms, err := splitPattern(pattern)
	if err != nil {
		return nil, nil, err
	}
	if len(terms) == 0 {
		return nil, nil, nil
	}

	result := map[string]*Host{}
	var unmatched []string
	for _, term := range orderPatterns(terms) {
		op, expr := termOp(term)
		matched, groupMatched, err := inv.matchTerm(expr)
		if err != nil {
			return nil, nil, err
		}
		if len(matched) == 0 && !groupMatched {
			unmatched = append(unmatched, expr)
		}
		switch op {
		case '!':
			for _, h := range matched {
				delete(result, h.Name)
			}
		case '&':
			keep := map[string]*Host{}
			for _, h := range matched {
				if _, ok := result[h.Name]; ok {
					keep[h.Name] = h
				}
			}
			result = keep
		default: // union
			for _, h := range matched {
				result[h.Name] = h
			}
		}
	}

	out := make([]*Host, 0, len(result))
	for _, h := range result {
		out = append(out, h)
	}
	// INVENTORY order, not alphabetical: real lists and runs hosts in
	// the order the inventory introduced them, which is what
	// "order: inventory" — the default — means. Sorting by name here
	// put `lonely` after `h1` where real puts it first.
	sort.Slice(out, func(i, j int) bool {
		return inv.HostIndex(out[i].Name) < inv.HostIndex(out[j].Name)
	})
	return out, unmatched, nil
}

// orderPatterns is real Ansible's own order_patterns
// (ansible/inventory/manager.py): the terms of a pattern are applied
// by KIND, not in the order they were written — every plain term
// first, then every `&` intersection, then every `!` exclusion. And a
// pattern with no plain term at all gets an implicit "all" to subtract
// from, which is what makes `!web3` mean "everything except web3"
// rather than "web3".
//
// Applying the terms left to right instead is not a near-miss: it
// INVERTS the common case. `hosts: "!db-primary"` selected exactly the
// host it names, so a play written to avoid one machine ran on that
// machine and nowhere else. Measured against real ansible-core 2.21.4,
// which reorders first.
func orderPatterns(terms []string) []string {
	var regular, intersect, exclude []string
	for _, t := range terms {
		if t == "" {
			continue
		}
		switch t[0] {
		case '!':
			exclude = append(exclude, t)
		case '&':
			intersect = append(intersect, t)
		default:
			regular = append(regular, t)
		}
	}
	if len(regular) == 0 {
		regular = []string{"all"}
	}

	out := make([]string, 0, len(regular)+len(intersect)+len(exclude))
	out = append(out, regular...)
	out = append(out, intersect...)
	return append(out, exclude...)
}

// termOp splits a leading '!' or '&' operator off a pattern term.
func termOp(term string) (byte, string) {
	if len(term) > 0 && (term[0] == '!' || term[0] == '&') {
		return term[0], term[1:]
	}
	return 0, term
}

// splitPattern tokenizes on top-level ':' and ',' — top-level meaning
// outside a '[...]' range, since a range itself may contain ':'
// (e.g. "web[01:10]").
func splitPattern(pattern string) ([]string, error) {
	var terms []string
	var cur strings.Builder
	depth := 0
	for _, r := range pattern {
		switch r {
		case '[':
			depth++
			cur.WriteRune(r)
		case ']':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("inventory: unbalanced ']' in pattern %q", pattern)
			}
			cur.WriteRune(r)
		case ':', ',':
			if depth == 0 {
				if t := strings.TrimSpace(cur.String()); t != "" {
					terms = append(terms, t)
				}
				cur.Reset()
				continue
			}
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("inventory: unbalanced '[' in pattern %q", pattern)
	}
	if t := strings.TrimSpace(cur.String()); t != "" {
		terms = append(terms, t)
	}
	return terms, nil
}

// matchTerm resolves ONE term of a pattern, and also reports whether
// that term named a GROUP — which is not the same question as whether
// it produced hosts, and real Ansible asks both. A term naming an
// empty group matches nothing yet is not a mistyped pattern, so
// _enumerate_matches (ansible/inventory/manager.py) warns only when a
// term matched neither a host NOR a group. Collapsing the two would
// warn about every empty group in the inventory.
func (inv *Inventory) matchTerm(term string) ([]*Host, bool, error) {
	// The IMPLICIT LOCALHOST: real always has one, even with an empty
	// or unreadable inventory, so `ansible localhost -m ping` and a
	// play written `hosts: localhost` work with no inventory at all.
	// It is NOT a member of any group, so "all" does not match it —
	// real even says so in a warning when that is what you asked for.
	if h := inv.implicitLocalhost(term); h != nil {
		return []*Host{h}, false, nil
	}
	// "all" and "*" always name a group: real exempts "all" from the
	// warning by name, and "*" escapes it by glob-matching the "all"
	// group itself, which every inventory has. Neither is ever a
	// mistyped pattern, however empty the inventory is.
	if term == "all" || term == "*" {
		out := make([]*Host, 0, len(inv.Hosts))
		for _, h := range inv.Hosts {
			out = append(out, h)
		}
		return out, true, nil
	}

	// Exact group name.
	if g, ok := inv.Groups[term]; ok {
		out := make([]*Host, 0, len(g.Hosts))
		seen := map[string]bool{}
		var collect func(*Group)
		collect = func(cur *Group) {
			for name, h := range cur.Hosts {
				if !seen[name] {
					seen[name] = true
					out = append(out, h)
				}
			}
			for _, c := range cur.Children {
				collect(c)
			}
		}
		collect(g)
		return out, true, nil
	}

	// Exact host name (fast path, before range/glob expansion).
	if h, ok := inv.Hosts[term]; ok {
		return []*Host{h}, false, nil
	}

	names, err := expandRanges(term)
	if err != nil {
		return nil, false, err
	}

	var out []*Host
	var groupMatched bool
	for _, name := range names {
		if h, ok := inv.Hosts[name]; ok {
			out = append(out, h)
			continue
		}
		if g, ok := inv.Groups[name]; ok {
			groupMatched = true
			for _, h := range g.Hosts {
				out = append(out, h)
			}
			continue
		}
		if strings.ContainsAny(name, "*?") {
			for hostName, h := range inv.Hosts {
				if ok, _ := path.Match(name, hostName); ok {
					out = append(out, h)
				}
			}
			for groupName, g := range inv.Groups {
				if ok, _ := path.Match(name, groupName); ok {
					groupMatched = true
					for _, h := range g.Hosts {
						out = append(out, h)
					}
				}
			}
		}
	}
	return out, groupMatched, nil
}

var rangePattern = regexp.MustCompile(`\[([0-9]+|[a-zA-Z]):([0-9]+|[a-zA-Z])(?::([0-9]+))?\]`)

// expandRanges expands one Ansible-style numeric or alphabetic range
// within a pattern term, e.g. "web[01:03].example.com" ->
// ["web01.example.com", "web02.example.com", "web03.example.com"], or
// "db[a:c]" -> ["dba", "dbb", "dbc"]. A term without a range expands to
// itself.
func expandRanges(term string) ([]string, error) {
	loc := rangePattern.FindStringSubmatchIndex(term)
	if loc == nil {
		return []string{term}, nil
	}
	prefix := term[:loc[0]]
	suffix := term[loc[1]:]
	start := term[loc[2]:loc[3]]
	end := term[loc[4]:loc[5]]
	step := 1
	if loc[6] >= 0 {
		s, err := strconv.Atoi(term[loc[6]:loc[7]])
		if err != nil || s <= 0 {
			return nil, fmt.Errorf("inventory: invalid range step in %q", term)
		}
		step = s
	}

	var mids []string
	if isDigits(start) && isDigits(end) {
		lo, _ := strconv.Atoi(start)
		hi, _ := strconv.Atoi(end)
		width := 0
		if len(start) > 1 || start[0] == '0' {
			width = len(start)
		}
		if lo <= hi {
			for i := lo; i <= hi; i += step {
				mids = append(mids, padNumber(i, width))
			}
		} else {
			for i := lo; i >= hi; i -= step {
				mids = append(mids, padNumber(i, width))
			}
		}
	} else if len(start) == 1 && len(end) == 1 {
		lo, hi := start[0], end[0]
		if lo <= hi {
			for c := lo; c <= hi; c++ {
				mids = append(mids, string(c))
			}
		} else {
			for c := lo; c >= hi; c-- {
				mids = append(mids, string(c))
			}
		}
	} else {
		return nil, fmt.Errorf("inventory: invalid range %q", term[loc[0]:loc[1]])
	}

	var out []string
	for _, mid := range mids {
		expanded, err := expandRanges(prefix + mid + suffix)
		if err != nil {
			return nil, err
		}
		out = append(out, expanded...)
	}
	return out, nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func padNumber(n int, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// implicitLocalhostNames are the three spellings real creates an
// implicit host for — measured, and CASE-SENSITIVE: "LOCALHOST" gets
// nothing.
var implicitLocalhostNames = map[string]bool{
	"localhost": true, "127.0.0.1": true, "::1": true,
}

// implicitLocalhost returns the host real invents for one of those
// names when the inventory does not define it, and nil otherwise —
// including when the inventory DOES define it, where the real entry
// wins and brings its own variables.
//
// It connects locally, which is the point: there is no ssh to
// localhost to configure.
func (inv *Inventory) implicitLocalhost(name string) *Host {
	if !implicitLocalhostNames[name] {
		return nil
	}
	if _, defined := inv.Hosts[name]; defined {
		return nil
	}
	return &Host{Name: name, Vars: map[string]any{"ansible_connection": "local"}}
}
