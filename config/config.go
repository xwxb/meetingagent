package config

import (
	"fmt"
	"io/ioutil"

	"github.com/cloudwego/eino/schema"
	"gopkg.in/yaml.v3"
)

type Config struct {
	APIKey    string         `yaml:"apikey"`
	BaseURL   string         `yaml:"base_url"`
	Summary   SummaryConfig  `yaml:"summary"`
	ChatAgent ChatAgent      `yaml:"chatagent"`
	Embedder  EmbedderConfig `yaml:"embedder"`
	Redis     RedisConfig    `yaml:"redis"`
}

type SummaryConfig struct {
	Model         string `yaml:"model"`
	SystemMessage string `yaml:"system_message"`
}

type ChatAgent struct {
	Model          string          `yaml:"model"`
	SystemMessage  string          `yaml:"system_message"`
	ChatSpecialist ChatSpecialists `yaml:"chat_specialist"`
}

type ChatSpecialists struct {
	TaskManagement SpecialistConfig `yaml:"task_management"`
	MeetingChat    SpecialistConfig `yaml:"meeting_chat"`
}

type SpecialistConfig struct {
	Model         string `yaml:"model"`
	SystemMessage string `yaml:"system_message"`
}

type EmbedderConfig struct {
	Model string `yaml:"model"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	Db       int    `yaml:"db"`
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

// GetSummarySystemMessage returns the system message for meeting summarization as a properly formatted schema.Message
func (c *Config) GetSummarySystemMessage() *schema.Message {
	return &schema.Message{
		Role:    schema.System,
		Content: c.Summary.SystemMessage,
	}
}

// GetChatAgentSystemMessage returns the system message for the chat agent as a properly formatted schema.Message
func (c *Config) GetChatAgentSystemMessage() *schema.Message {
	return &schema.Message{
		Role:    schema.System,
		Content: c.ChatAgent.SystemMessage,
	}
}

// GetSpecialistSystemMessage returns the system message for a specific chat specialist
func (c *Config) GetSpecialistSystemMessage(specialist string) (*schema.Message, error) {
	var systemMessage string
	switch specialist {
	case "task_management":
		systemMessage = c.ChatAgent.ChatSpecialist.TaskManagement.SystemMessage
	case "meeting_chat":
		systemMessage = c.ChatAgent.ChatSpecialist.MeetingChat.SystemMessage
	default:
		return nil, fmt.Errorf("unknown specialist: %s", specialist)
	}

	return &schema.Message{
		Role:    schema.System,
		Content: systemMessage,
	}, nil
}
