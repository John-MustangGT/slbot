package config

import (
	"encoding/xml"
	"io"
	"os"
)

// Config holds all configuration settings
type Config struct {
	XMLName xml.Name      `xml:"config"`
	Corrade CorradeConfig `xml:"corrade"`
	Llama   LlamaConfig   `xml:"llama"`
	Bot     BotConfig     `xml:"bot"`
	Prompts PromptsConfig `xml:"prompts"`
}

// CorradeConfig holds Corrade connection settings
type CorradeConfig struct {
	URL      string `xml:"url"`
	Group    string `xml:"group"`
	Password string `xml:"password"`
}

// LlamaConfig holds Llama API settings
type LlamaConfig struct {
	Enabled bool   `xml:"enabled"`
	URL     string `xml:"url"`
	Model   string `xml:"model"`
}

// BotConfig holds bot-specific settings
type BotConfig struct {
	Name                    string   `xml:"name"`
	MaxMessageLen           int      `xml:"maxMessageLen"`
	PollInterval            int      `xml:"pollInterval"`
	ResponseTimeout         int      `xml:"responseTimeout"`
	WebPort                 int      `xml:"webPort"`
	IdleTimeout             int      `xml:"idleTimeout"`             // Minutes before idle behavior
	IdleBehaviorMinInterval int      `xml:"idleBehaviorMinInterval"` // Minimum minutes between idle behaviors
	IdleBehaviorMaxInterval int      `xml:"idleBehaviorMaxInterval"` // Maximum minutes between idle behaviors
	Owners                  []string `xml:"owners>owner"`
}

// PromptsConfig holds various prompts for different situations
type PromptsConfig struct {
	SystemPrompt      string            `xml:"systemPrompt"`
	ChatPrompt        string            `xml:"chatPrompt"`
	WelcomeMessage    string            `xml:"welcomeMessage"`
	ErrorMessage      string            `xml:"errorMessage"`
	GreetingPrompt    string            `xml:"greetingPrompt"`
	HelpPrompt        string            `xml:"helpPrompt"`
	FallbackResponses FallbackResponses `xml:"fallbackResponses"`
}

// FallbackResponses holds predefined responses when AI is disabled
type FallbackResponses struct {
	Greeting string `xml:"greeting"`
	Help     string `xml:"help"`
	General  string `xml:"general"`
	Unknown  string `xml:"unknown"`
}

// Load loads configuration from XML file
func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := xml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// GetIdleBehaviorMinInterval returns the minimum idle behavior interval
func (c *Config) GetIdleBehaviorMinInterval() int {
	return c.Bot.IdleBehaviorMinInterval
}

// GetIdleBehaviorMaxInterval returns the maximum idle behavior interval
func (c *Config) GetIdleBehaviorMaxInterval() int {
	return c.Bot.IdleBehaviorMaxInterval
}
