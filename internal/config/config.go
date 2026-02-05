package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/ini.v1"
)

type SlackConfig struct {
	ClientID  string
	BotToken  string
	ChannelID string
}

type LLMConfig struct {
	Provider      string
	Model         string
	Temperature   float64
	TopP          float64
	RepeatPenalty float64
	ContextSize   int
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

	slackSec := getSection(cfg, "slack", "Slack")
	llmSec := getSection(cfg, "llm", "LLM")

	return &Config{
		Slack: SlackConfig{
			ClientID:  getKey(slackSec, "client_id", "ClientID", "Client_Id").String(),
			BotToken:  getKey(slackSec, "bot_token", "BotToken", "Bot_Token").String(),
			ChannelID: getKey(slackSec, "channel_id", "ChannelID", "Channel_Id").String(),
		},
		LLM: LLMConfig{
			Provider:      getKey(llmSec, "provider", "Provider").MustString("ollama"),
			Model:         strings.Trim(getKey(llmSec, "model", "Model").MustString("llama3.2"), "\""),
			Temperature:   getKey(llmSec, "temperature", "Temperature").MustFloat64(0.7),
			TopP:          getKey(llmSec, "top_p", "TopP").MustFloat64(0.9),
			RepeatPenalty: getKey(llmSec, "repeat_penalty", "RepeatPenalty").MustFloat64(1.1),
			ContextSize:   getKey(llmSec, "context_size", "ContextSize", "num_ctx").MustInt(8192),
			BaseURL:       strings.Trim(getKey(llmSec, "base_url", "BaseUrl", "BaseURL").MustString("http://localhost:11434/api/generate"), "\""),
		},
	}, nil
}

func getSection(cfg *ini.File, names ...string) *ini.Section {
	for _, name := range names {
		if sec, err := cfg.GetSection(name); err == nil {
			return sec
		}
	}
	return cfg.Section(names[0])
}

func getKey(sec *ini.Section, keys ...string) *ini.Key {
	for _, k := range keys {
		if key, err := sec.GetKey(k); err == nil {
			return key
		}
	}
	// Return the first one so Must* functions can handle the default on it
	return sec.Key(keys[0])
}
