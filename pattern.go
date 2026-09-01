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
// against the inventory, and returns the matching hosts sorted by name.
func (inv *Inventory) Match(pattern string) ([]*Host, error) {
	terms, err := splitPattern(pattern)
	if err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		return nil, nil
	}

	result := map[string]*Host{}
	for i, term := range terms {
		op, expr := termOp(term)
		matched, err := inv.matchTerm(expr)
		if err != nil {
			return nil, err
		}
		switch {
		case i == 0:
			for _, h := range matched {
				result[h.Name] = h
			}
		case op == '!':
			for _, h := range matched {
				delete(result, h.Name)
			}
		case op == '&':
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
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
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

func (inv *Inventory) matchTerm(term string) ([]*Host, error) {
	if term == "all" || term == "*" {
		out := make([]*Host, 0, len(inv.Hosts))
		for _, h := range inv.Hosts {
			out = append(out, h)
		}
		return out, nil
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
		return out, nil
	}

	// Exact host name (fast path, before range/glob expansion).
	if h, ok := inv.Hosts[term]; ok {
		return []*Host{h}, nil
	}

	names, err := expandRanges(term)
	if err != nil {
		return nil, err
	}

	var out []*Host
	for _, name := range names {
		if h, ok := inv.Hosts[name]; ok {
			out = append(out, h)
			continue
		}
		if g, ok := inv.Groups[name]; ok {
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
					for _, h := range g.Hosts {
						out = append(out, h)
					}
				}
			}
		}
	}
	return out, nil
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
