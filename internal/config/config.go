package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// OAuthProfile is generic OAuth2 endpoints for a credential profile.
type OAuthProfile struct {
	AuthorizeURL       string   `yaml:"authorize_url"`
	TokenURL           string   `yaml:"token_url"`
	DeviceAuthorizeURL string   `yaml:"device_authorize_url,omitempty"`
	DeviceTokenURL     string   `yaml:"device_token_url,omitempty"`
	ClientID           string   `yaml:"client_id,omitempty"`
	Scopes             []string `yaml:"scopes,omitempty"`
	PKCE               bool     `yaml:"pkce,omitempty"`
}

const BrokerKeyEnv = "CINCAI_BROKER_KEY"

// Profile is a credential profile definition.
type Profile struct {
	Kind  string       `yaml:"kind"`
	OAuth OAuthProfile `yaml:"oauth,omitempty"`
}

// File is cincai.yaml root.
type File struct {
	Serve struct {
		DataListen string `yaml:"data_listen"`
		Catalog    string `yaml:"catalog"`
	} `yaml:"serve"`
	Credential struct {
		Backend    string `yaml:"backend,omitempty"`
		Broker     string `yaml:"broker"`
		Encryption struct {
			KeyEnv  string `yaml:"key_env"`
			KeyFile string `yaml:"key_file"`
		} `yaml:"encryption"`
		BackendConfig yaml.Node `yaml:"backend_config,omitempty"`
	} `yaml:"credential"`
	Adapters struct {
		Enable []string `yaml:"enable"`
	} `yaml:"adapters"`
}

// Load reads and normalizes cincai.yaml.
func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var probe struct {
		CredentialProfiles map[string]any `yaml:"credential_profiles"`
	}
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	if len(probe.CredentialProfiles) > 0 {
		return nil, fmt.Errorf("credential_profiles in cincai.yaml is not supported; use cincai credential login or credential import (profile names match providers.yaml credential_profile)")
	}

	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	if f.Serve.DataListen == "" {
		// Secure by default: bind loopback only. The data plane carries gateway API
		// keys in cleartext, so exposing it beyond the local host is an explicit
		// operator choice (set data_listen to a specific interface and front it with
		// TLS). Reaching it from other containers means sharing the pod's network
		// namespace or publishing the port deliberately.
		f.Serve.DataListen = "127.0.0.1:9420"
	}
	if f.Serve.Catalog == "" {
		f.Serve.Catalog = "providers.yaml"
	}
	if f.Credential.Backend == "" {
		f.Credential.Backend = "sqlite"
	}
	if f.Credential.Broker == "" {
		f.Credential.Broker = "broker.db"
	}
	if f.Credential.Encryption.KeyEnv == "" {
		f.Credential.Encryption.KeyEnv = BrokerKeyEnv
	}
	if len(f.Adapters.Enable) == 0 {
		f.Adapters.Enable = []string{"passthrough"}
	}
	return &f, nil
}

// BrokerKey reads the broker.db AES encryption key from env or file.
func BrokerKey(f *File) (string, error) {
	if f.Credential.Encryption.KeyFile != "" {
		path := f.Credential.Encryption.KeyFile
		if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o077 != 0 {
			// The broker key decrypts every stored credential — warn if anyone but the
			// owner can read the file (mirrors the 0600 broker.db handling).
			slog.Warn("broker key file is group/world-readable; restrict to 0600",
				"key_file", path, "mode", fmt.Sprintf("%04o", info.Mode().Perm()))
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(raw)), nil
	}
	envName := f.Credential.Encryption.KeyEnv
	v := strings.TrimSpace(os.Getenv(envName))
	if v == "" {
		return "", fmt.Errorf("broker encryption key required: set %s or credential.encryption.key_file", envName)
	}
	return v, nil
}
