package config

import (
	"fmt"
	"os"
	"regexp"

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
	Name  string        `toml:"name"`
	Layer []LayerConfig `toml:"layer"`
}

type AzureConfig struct {
	ConnectionString string `toml:"connection_string"`
	Container        string `toml:"container"`
	PrefixKey        string `toml:"prefix_key"`
}

type FileConfig struct {
	Base string `toml:"base"`
}

// In case S3 will be needed in the future, these are the old `t_rex` configs.
// type S3Config struct {
// 	Endpoint  string `toml:"endpoint"`
// 	Bucket    string `toml:"bucket"`
// 	AccessKey string `toml:"access_key"`
// 	SecretKey string `toml:"secret_key"`
// 	Region    string `toml:"region"`
// 	KeyPrefix string `toml:"key_prefix"`
// }

type CacheConfig struct {
	File  *FileConfig  `toml:"file"`
	Azure *AzureConfig `toml:"azure"`
}

// TomlConfig mirrors the top-level structure of an mvt config toml file,
// e.g. example/NetherlandsRDNewQuad.toml.
type TomlConfig struct {
	DataSource []DataSource `toml:"datasource"`
	Tileset    *Tileset     `toml:"tileset"`
	Cache      CacheConfig  `toml:"cache"`
}

// Replace {{env.NAME}} by value of environment variable NAME. Inserted
// verbatim. Unset variables result in an error.
func substituteEnv(s string) (string, error) {
	envPlaceholder := regexp.MustCompile(
		`\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

	var missing []string
	out := envPlaceholder.ReplaceAllStringFunc(s, func(m string) string {
		name := envPlaceholder.FindStringSubmatch(m)[1]
		val, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
		}
		return val
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("unset environment variables: %v", missing)
	}
	return out, nil
}

// ParseMVTConfig reads an mvt config toml file, substitutes {{env.NAME}}
// placeholders and returns the resulting TomlConfig.
func ParseMVTConfig(path string) (TomlConfig, error) {
	var cfg TomlConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return TomlConfig{}, fmt.Errorf("reading mvt config %q: %w", path, err)
	}
	text, err := substituteEnv(string(raw))
	if err != nil {
		return TomlConfig{}, fmt.Errorf("mvt config %q: %w", path, err)
	}
	if _, err := toml.Decode(text, &cfg); err != nil {
		return TomlConfig{}, fmt.Errorf("decoding mvt config %q: %w", path, err)
	}
	return cfg, nil
}
