# inventory

Ansible-compatible inventory: INI/YAML parsers, groups, host/group vars, patterns.

Part of [go-ansible](https://github.com/go-ansible) — a pure-Go (CGO=0),
functional-parity port of [Ansible](https://www.ansible.com/).

[![CI](https://github.com/go-ansible/inventory/actions/workflows/ci.yml/badge.svg)](https://github.com/go-ansible/inventory/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-ansible/inventory.svg)](https://pkg.go.dev/github.com/go-ansible/inventory)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)

## Usage

```go
inv, err := inventory.Load("inventory.ini") // INI or YAML, group_vars/host_vars included

hosts, err := inv.Match("webservers:&staging:!maintenance") // Ansible host-pattern syntax
vars := inv.HostVars("web1.example.com")                    // group + host var precedence applied
groups := inv.GroupsForHost("web1.example.com")              // full ancestry, not just direct membership
```

`Merge` combines a second `*Inventory` (e.g. a dynamic-inventory result) into
an existing one.
