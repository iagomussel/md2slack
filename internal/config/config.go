package config

import (
	"os"
	"path/filepath"

	"gopkg.in/ini.v1"
)

type SlackConfig struct {
	ClientID  string
	BotToken  string
	ChannelID string
}

type LLMConfig struct {
	Model         string
	Temperature   float64
	TopP          float64
	RepeatPenalty float64
	BaseURL       string
}

type Config struct {
	Slack SlackConfig
	LLM   LLMConfig
}

func Load() (*Config, error) {
	configPath := "config.ini"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		home, err := os.UserHomeDir()
		if err == nil {
			globalPath := filepath.Join(home, ".md2slack", "config.ini")
			if _, err := os.Stat(globalPath); err == nil {
				configPath = globalPath
			}
		}
	}

	cfg, err := ini.Load(configPath)
	if err != nil {
		return nil, err
	}

	slackSec := cfg.Section("slack")
	llmSec := cfg.Section("llm")

	return &Config{
		Slack: SlackConfig{
			ClientID:  slackSec.Key("client_id").String(),
			BotToken:  slackSec.Key("bot_token").String(),
			ChannelID: slackSec.Key("channel_id").String(),
		},
		LLM: LLMConfig{
			Model:         llmSec.Key("model").MustString("llama3.2"),
			Temperature:   llmSec.Key("temperature").MustFloat64(0.7),
			TopP:          llmSec.Key("top_p").MustFloat64(0.9),
			RepeatPenalty: llmSec.Key("repeat_penalty").MustFloat64(1.1),
			BaseURL:       llmSec.Key("base_url").MustString("http://localhost:11434/api/generate"),
		},
	}, nil
}
