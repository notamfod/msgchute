package sender

import "time"

type Config struct {
	MaxBufferSize       int                        `yaml:"maxBufferSize"`
	FetchTaskTimeout    time.Duration              `yaml:"fetchTaskTimeout"`
	FetchTaskTimeoutMax time.Duration              `yaml:"fetchTaskTimeoutMax"`
	PluginMap           map[string]string          `yaml:"pluginMap"`
	Providers           map[string]*ProviderConfig `yaml:"providers"`
}

type ProviderConfig struct {
	Provider        string          `yaml:"provider"`
	Channel         string          `yaml:"channel"`
	RetrierSettings RetrierSettings `yaml:"settings"`
	Params          map[string]any  `yaml:"params"`
}

type RetrierSettings struct {
	Workers struct {
		Min int `yaml:"min"`
		Max int `yaml:"max"`
	} `yaml:"workers"`
	Breaker struct {
		WindowSize       time.Duration `yaml:"windowSize"`
		MinRequests      int           `yaml:"minRequests"`
		FailureThreshold float64       `yaml:"failureThreshold"`
		Timeout          time.Duration `yaml:"timeout"`
	} `yaml:"breaker"`
}
