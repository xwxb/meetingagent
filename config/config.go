package config

import (
	"fmt"
	"io/ioutil"

	"github.com/cloudwego/eino/schema"
	"gopkg.in/yaml.v3"
)

type Config struct {
	APIKey  string        `yaml:"apikey"`
	BaseURL string        `yaml:"base_url"`
	Summary SummaryConfig `yaml:"summary"`
}

type SummaryConfig struct {
	Model         string `yaml:"model"`
	SystemMessage string `yaml:"system_message"`
}

var AppConfig *Config

// LoadConfig loads configuration from the specified YAML file
func LoadConfig(configPath string) (*Config, error) {
	yamlFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	var config Config
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return nil, fmt.Errorf("error parsing config file: %v", err)
	}

	AppConfig = &config
	return &config, nil
}

// GetSystemMessage returns the system message for meeting summarization as a properly formatted schema.Message
func (c *Config) GetSummarySystemMessage() *schema.Message {
	return &schema.Message{
		Role:    schema.System,
		Content: c.Summary.SystemMessage,
	}
}
