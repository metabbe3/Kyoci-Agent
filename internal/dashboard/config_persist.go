package dashboard

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// persistProviderUpdates reads cfgPath, applies the supplied provider updates,
// writes the original to cfgPath.backup, then atomically replaces cfgPath
// via a .tmp + rename. Comments and unrelated sections are preserved because
// we round-trip through yaml.Node rather than re-marshaling a struct.
func persistProviderUpdates(cfgPath string, updates map[string]ProviderConfigDTO) error {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse config YAML: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return errors.New("config root is not a YAML document")
	}

	topMap := root.Content[0]
	if topMap.Kind != yaml.MappingNode {
		return errors.New("config top level is not a mapping")
	}

	providersMap := findMapEntry(topMap, "providers")
	if providersMap == nil {
		// providers section missing — create it
		providersMap = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		topMap.Content = append(topMap.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "providers"},
			providersMap,
		)
	}

	for name, dto := range updates {
		provNode := findMapEntry(providersMap, name)
		if provNode == nil {
			provNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			providersMap.Content = append(providersMap.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
				provNode,
			)
		}
		applyProviderDTO(provNode, dto)
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("re-marshal config: %w", err)
	}

	// Backup the original (overwrites any prior .backup — v1 keeps a single
	// rollback point; full versioned history can come later).
	if err := os.WriteFile(cfgPath+".backup", data, 0o644); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}

	// Atomic replace via tmp + rename.
	tmp := cfgPath + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, cfgPath); err != nil {
		return fmt.Errorf("rename tmp over config: %w", err)
	}
	return nil
}

// findMapEntry returns the value node for a key in a mapping, or nil.
func findMapEntry(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// applyProviderDTO writes a DTO's fields into a provider mapping node.
// For api_key: empty or MaskedAPIKey means "leave alone" — only real values
// overwrite. This way the UI can round-trip the masked placeholder without
// clobbering the real key on disk.
func applyProviderDTO(provNode *yaml.Node, dto ProviderConfigDTO) {
	setBool(provNode, "enabled", dto.Enabled)
	if dto.BaseURL != "" {
		setString(provNode, "base_url", dto.BaseURL)
	}
	if dto.DefaultModel != "" {
		setString(provNode, "default_model", dto.DefaultModel)
	}
	if dto.APIKey != "" && dto.APIKey != MaskedAPIKey {
		setString(provNode, "api_key", dto.APIKey)
	}
	if dto.Timeout > 0 {
		setInt(provNode, "timeout", dto.Timeout)
	}
	if dto.MaxRetries > 0 {
		setInt(provNode, "max_retries", dto.MaxRetries)
	}
}

func setString(mapping *yaml.Node, key, value string) {
	setScalar(mapping, key, value, "!!str")
}

func setBool(mapping *yaml.Node, key string, value bool) {
	v := "false"
	if value {
		v = "true"
	}
	setScalar(mapping, key, v, "!!bool")
}

func setInt(mapping *yaml.Node, key string, value int) {
	setScalar(mapping, key, fmt.Sprintf("%d", value), "!!int")
}

// setScalar upserts a key/value pair into a mapping node. If the key exists,
// its value node is replaced in place (preserving its position); otherwise a
// new key+value pair is appended.
func setScalar(mapping *yaml.Node, key, value, tag string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			valNode := mapping.Content[i+1]
			valNode.Kind = yaml.ScalarNode
			valNode.Tag = tag
			valNode.Value = value
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value},
	)
}
