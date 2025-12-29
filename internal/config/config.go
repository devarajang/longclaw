package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	TLS      TLSConfig
	Data     DataConfig
}
type DataConfig struct {
	SpecPath      string
	CardsPath     string
	TemplatesPath string
	Path          string
}
type ServerConfig struct {
	HTTPPort string
	ISOPort  string
}

type DatabaseConfig struct {
	Path string
}

type TLSConfig struct {
	ServerCertPath string
	ServerKeyPath  string
	CAPath         string
	ClientCertPath string
	ClientKeyPath  string
	ExpectedCN     string
}

func Load(basePath string) (*Config, error) {
	dataPath := filepath.Join(basePath, "data")
	certsPath := filepath.Join(basePath, "certs")

	cfg := &Config{

		Server: ServerConfig{
			HTTPPort: getEnv("LONGCLAW_HTTP_PORT", ":8080"),
			ISOPort:  getEnv("LONGCLAW_ISO_PORT", ":8443"),
		},
		Database: DatabaseConfig{
			Path: getEnv("LONGCLAW_DB_PATH", filepath.Join(dataPath, "stress_test.db")),
		},
		TLS: TLSConfig{
			ServerCertPath: getEnv("LONGCLAW_SERVER_CERT", filepath.Join(certsPath, "server", "server.crt")),
			ServerKeyPath:  getEnv("LONGCLAW_SERVER_KEY", filepath.Join(certsPath, "server", "server.key")),
			CAPath:         getEnv("LONGCLAW_CA_CERT", filepath.Join(certsPath, "server", "ca.crt")),
			ClientCertPath: getEnv("LONGCLAW_CLIENT_CERT", filepath.Join(certsPath, "client", "client.crt")),
			ClientKeyPath:  getEnv("LONGCLAW_CLIENT_KEY", filepath.Join(certsPath, "client", "client.key")),
			ExpectedCN:     getEnv("LONGCLAW_EXPECTED_CN", "photon-client"),
		},
		Data: DataConfig{
			Path:          dataPath,
			SpecPath:      getEnv("LONGCLAW_SPEC_PATH", filepath.Join(dataPath, "file_spec.json")),
			CardsPath:     getEnv("LONGCLAW_CARDS_PATH", filepath.Join(dataPath, "test_cards.json")),
			TemplatesPath: getEnv("LONGCLAW_TEMPLATES_PATH", filepath.Join(dataPath, "template_messages.json")),
		},
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
