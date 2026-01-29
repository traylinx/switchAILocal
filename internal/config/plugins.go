package config

// PluginConfig holds LUA plugin system settings.
type PluginConfig struct {
	// Enabled toggles the LUA plugin system.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// PluginDir is the directory containing LUA scripts.
	PluginDir string `yaml:"plugin-dir" json:"plugin-dir"`

	// EnabledPlugins specifies a list of LUA plugin IDs to load.
	// Only plugins with a matching 'name' variable will be loaded.
	EnabledPlugins []string `yaml:"enabled-plugins" json:"enabled-plugins"`
}
