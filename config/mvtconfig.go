package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type DataSource struct {
	Name string `toml:"name"`
	Path string `toml:"path"`
}

type LayerConfig struct {
	Name       string `toml:"name"`
	MinZoom    uint   `toml:"minzoom"`
	MaxZoom    uint   `toml:"maxzoom"`
	DataSource string `toml:"datasource"`
	TableName  string `toml:"table_name"`
}

// Tileset mirrors a `[[Tileset]]` array-of-tables entry, including its
// nested `[[Tileset.layer]]` entries.
type Tileset struct {
	Name    string        `toml:"name"`
	MinZoom uint          `toml:"minzoom"`
	MaxZoom uint          `toml:"maxzoom"`
	Layer   []LayerConfig `toml:"layer"`
}

// TomlConfig mirrors the top-level structure of an mvt config toml file,
// e.g. example/NetherlandsRDNewQuad.toml.
type TomlConfig struct {
	DataSource []DataSource `toml:"datasource"`
	Tileset    []Tileset    `toml:"tileset"`
}

// ParseMVTConfig reads an mvt config toml file and returns the resulting
// TomlConfig.
func ParseMVTConfig(path string) (TomlConfig, error) {
	var cfg TomlConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return TomlConfig{}, fmt.Errorf("decoding mvt config %q: %w", path, err)
	}
	return cfg, nil
}
