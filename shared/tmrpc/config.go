package tmrpc

import (
	"os"

	"gopkg.in/yaml.v3"
)

type TendermintRPCClientManagerConfig struct {
	Remotes map[string]Remote `yaml:"remotes"`
}

type Remote struct {
	Endpoint string `yaml:"endpoint"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

func LoadConfig(path string) (TendermintRPCClientManagerConfig, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return TendermintRPCClientManagerConfig{}, err
	}
	var config TendermintRPCClientManagerConfig
	if err := yaml.Unmarshal(file, &config); err != nil {
		return TendermintRPCClientManagerConfig{}, err
	}
	return config, nil
}
