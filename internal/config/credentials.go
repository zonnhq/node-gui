package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Credentials are this node's identity with one Coordinator, issued by
// enrollment. They live in their own file, readable only by the user, and are
// never part of Config, so the local API (which returns Config) can never
// expose them.
type Credentials struct {
	NodeID string `json:"node_id"`
	Secret string `json:"node_secret"`
	// CoordinatorURL is the Coordinator that issued the credential. It is only
	// ever sent there: if the configured address changes, the node must enroll
	// again rather than hand its secret to a different server.
	CoordinatorURL string `json:"coordinator_url"`
}

func credentialsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

// LoadCredentials returns the stored credentials, or false if the node has not
// enrolled.
func LoadCredentials() (Credentials, bool) {
	path, err := credentialsPath()
	if err != nil {
		return Credentials{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, false
	}
	var c Credentials
	if json.Unmarshal(data, &c) != nil || c.NodeID == "" || c.Secret == "" {
		return Credentials{}, false
	}
	return c, true
}

// SaveCredentials writes the credentials atomically with owner only access.
func SaveCredentials(c Credentials) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ClearCredentials forgets the credentials, for example when the Coordinator
// no longer accepts them.
func ClearCredentials() error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
