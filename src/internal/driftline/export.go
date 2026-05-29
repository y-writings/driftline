package driftline

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type PullRepo struct {
	Repo  string
	Units []string
}

func LoadPull(path string) ([]PullRepo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pull manifest: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse pull manifest: %w", err)
	}
	root := &doc
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("pull manifest must be mapping")
	}
	pullNode := mappingValue(root, "pull")
	if pullNode == nil || pullNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("pull manifest must define pull list")
	}
	var out []PullRepo
	for _, item := range pullNode.Content {
		if item.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("pull entries must be mappings")
		}
		repo := PullRepo{}
		for i := 0; i < len(item.Content)-1; i += 2 {
			k, v := item.Content[i].Value, item.Content[i+1]
			if k == "repo" {
				repo.Repo = v.Value
				continue
			}
			if v.Kind == yaml.SequenceNode {
				for _, n := range v.Content {
					repo.Units = append(repo.Units, n.Value)
				}
			} else {
				repo.Units = append(repo.Units, k)
			}
		}
		if repo.Repo == "" {
			return nil, fmt.Errorf("pull entry must define repo")
		}
		out = append(out, repo)
	}
	return out, nil
}

func LoadExportUnits(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read export manifest: %w", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse export manifest: %w", err)
	}
	n := &root
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		n = root.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("export manifest must be mapping")
	}
	out := map[string][]string{}
	for i := 0; i < len(n.Content)-1; i += 2 {
		name := n.Content[i].Value
		paths, err := flattenPaths("", n.Content[i+1])
		if err != nil {
			return nil, fmt.Errorf("export %q: %w", name, err)
		}
		out[name] = paths
	}
	return out, nil
}

func flattenPaths(prefix string, n *yaml.Node) ([]string, error) {
	var out []string
	switch n.Kind {
	case yaml.SequenceNode:
		for _, c := range n.Content {
			items, err := flattenPaths(prefix, c)
			if err != nil { return nil, err }
			out = append(out, items...)
		}
	case yaml.ScalarNode:
		out = append(out, filepath.Clean(filepath.Join(prefix, n.Value)))
	case yaml.MappingNode:
		for i := 0; i < len(n.Content)-1; i += 2 {
			k := n.Content[i].Value
			items, err := flattenPaths(filepath.Join(prefix, k), n.Content[i+1])
			if err != nil { return nil, err }
			out = append(out, items...)
		}
	default:
		return nil, fmt.Errorf("unsupported yaml node kind %d", n.Kind)
	}
	return out, nil
}

func mappingValue(n *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == key { return n.Content[i+1] }
	}
	return nil
}
