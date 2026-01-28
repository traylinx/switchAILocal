package config

// PluginConfig holds LUA plugin system settings.
type PluginConfig struct {
	// Enabled toggles the LUA plugin system.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// PluginDir is the directory containing LUA scripts.
	PluginDir string `yaml:"plugin-dir" json:"plugin-dir"`
}
