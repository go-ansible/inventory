package inventory

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// ParseINI parses an Ansible INI-style inventory:
//
//	host1 ansible_host=192.0.2.1
//
//	[webservers]
//	web[01:02].example.com http_port=80
//
//	[webservers:vars]
//	ansible_user=deploy
//
//	[webservers:children]
//	edge
func ParseINI(data []byte) (*Inventory, error) {
	inv := New()

	type section struct {
		group string
		kind  string // "hosts" (default), "vars", "children"
	}
	cur := section{group: "ungrouped", kind: "hosts"}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			end := strings.Index(line, "]")
			if end < 0 {
				return nil, fmt.Errorf("inventory: line %d: unterminated section header %q", lineNo, line)
			}
			header := line[1:end]
			if i := strings.LastIndex(header, ":"); i >= 0 && (header[i+1:] == "vars" || header[i+1:] == "children") {
				cur = section{group: header[:i], kind: header[i+1:]}
			} else {
				cur = section{group: header, kind: "hosts"}
			}
			inv.group(cur.group)
			continue
		}

		switch cur.kind {
		case "children":
			names, err := expandRanges(line)
			if err != nil {
				return nil, fmt.Errorf("inventory: line %d: %w", lineNo, err)
			}
			for _, name := range names {
				inv.addChild(cur.group, name)
			}

		case "vars":
			key, val, err := parseKV(line)
			if err != nil {
				return nil, fmt.Errorf("inventory: line %d: %w", lineNo, err)
			}
			inv.group(cur.group).Vars[key] = val

		default: // "hosts"
			fields := splitHostLine(line)
			if len(fields) == 0 {
				continue
			}
			names, err := expandRanges(fields[0])
			if err != nil {
				return nil, fmt.Errorf("inventory: line %d: %w", lineNo, err)
			}
			vars := map[string]any{}
			for _, f := range fields[1:] {
				key, val, err := parseKV(f)
				if err != nil {
					return nil, fmt.Errorf("inventory: line %d: %w", lineNo, err)
				}
				vars[key] = val
			}
			for _, name := range names {
				h := inv.addHostToGroup(name, cur.group)
				for k, v := range vars {
					h.Vars[k] = v
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("inventory: %w", err)
	}

	inv.finalize()
	return inv, nil
}

// splitHostLine splits a host-definition line into fields, honoring
// single/double-quoted values so an embedded space doesn't split a
// var=value pair (e.g. `host1 description="edge node"`).
func splitHostLine(line string) []string {
	var fields []string
	var cur strings.Builder
	var quote rune
	for _, r := range line {
		switch {
		case quote != 0:
			cur.WriteRune(r)
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
			cur.WriteRune(r)
		case r == ' ' || r == '\t':
			if cur.Len() > 0 {
				fields = append(fields, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		fields = append(fields, cur.String())
	}
	return fields
}

// parseKV parses a single `key=value` pair, unquoting the value and
// coercing it to bool/int/float where it unambiguously looks like one
// (matching Ansible's INI var typing).
func parseKV(field string) (string, any, error) {
	i := strings.IndexByte(field, '=')
	if i < 0 {
		return "", nil, fmt.Errorf("expected key=value, got %q", field)
	}
	key := strings.TrimSpace(field[:i])
	raw := strings.TrimSpace(field[i+1:])
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0] {
		return key, raw[1 : len(raw)-1], nil
	}
	return key, coerce(raw), nil
}

func coerce(raw string) any {
	switch raw {
	case "true", "True", "TRUE":
		return true
	case "false", "False", "FALSE":
		return false
	}
	if i, err := strconv.Atoi(raw); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f
	}
	return raw
}
