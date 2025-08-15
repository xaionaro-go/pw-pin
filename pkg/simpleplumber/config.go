package simpleplumber

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Routes Routes
}

func (cfg *Config) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	return nil
}

func (cfg *Config) Bytes() ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}
	return data, nil
}

type Routes []Route

type FullyQualifiedPortSelector struct {
	Node Constraints `yaml:"node,omitempty"`
	Port Constraints `yaml:"port,omitempty"`
}

type Route struct {
	From           FullyQualifiedPortSelector `yaml:"from,omitempty"`
	To             FullyQualifiedPortSelector `yaml:"to,omitempty"`
	ShouldBeLinked bool                       `yaml:"should_be_linked"`
}
