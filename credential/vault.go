// Package credential implements cincai credential admin commands (encrypted broker).
package credential

import (
	"path/filepath"

	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/gateway"
)

// Vault is an open encrypted credential store + router gateway config.
type Vault struct {
	Config *gateway.ConfigFile
	Store  store.Store
	close  func()
}

// LoadVault opens the encrypted broker from router.yaml (paths relative to config dir).
func LoadVault(configPath string) (*Vault, error) {
	cfgFile, err := gateway.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	base := filepath.Dir(configPath)
	brokerPath := cfgFile.Credential.Broker
	if !filepath.IsAbs(brokerPath) {
		brokerPath = filepath.Join(base, brokerPath)
	}
	cfgFile.Credential.Broker = brokerPath
	st, _, err := gateway.OpenStore(cfgFile)
	if err != nil {
		return nil, err
	}
	return &Vault{
		Config: cfgFile,
		Store:  st,
		close:  func() { _ = st.Close() },
	}, nil
}

// Close releases the store handle.
func (v *Vault) Close() {
	if v != nil && v.close != nil {
		v.close()
	}
}
