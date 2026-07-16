package gateway

import icfg "github.com/subosito/cincai/internal/config"

// Config types re-exported for operator binaries and linked plugins.
type (
	ConfigFile   = icfg.File
	Profile      = icfg.Profile
	OAuthProfile = icfg.OAuthProfile
)

// LoadConfig reads cincai.yaml.
func LoadConfig(path string) (*ConfigFile, error) {
	return icfg.Load(path)
}

// BrokerEncryptionKey returns the broker.db encryption key from config/env.
func BrokerEncryptionKey(f *ConfigFile) (string, error) {
	return icfg.BrokerKey(f)
}
