package main

import (
	"flag"
	"fmt"
	"md2slack/internal/config"
	"md2slack/internal/llm"
	"os"
	"time"
)

func main() {
	var install bool
	var debug bool
	flag.BoolVar(&install, "install", false, "Install the binary")
	flag.BoolVar(&debug, "debug", false, "Enable debug mode")
	flag.Parse()

	if install {
		runInstall()
		return
	}

	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 2. Define Stages for UI
	stageNames := []string{
		"Preparing commit context",
		"Summarizing commits",
		"Generating tasks",
		"Reviewing tasks",
		"Suggesting next actions",
		"Rendering report",
	}

	// 3. Initialize Web Server (CLI wrapper)
	webServer := startWebServer(cfg, stageNames)

	// 4. Initialize Core Processor
	processor := &ReportProcessor{
		Config: cfg,
		LLMOpts: llm.LLMOptions{
			Provider:      cfg.LLM.Provider,
			ModelName:     cfg.LLM.Model,
			Temperature:   cfg.LLM.Temperature,
			TopP:          cfg.LLM.TopP,
			RepeatPenalty: cfg.LLM.RepeatPenalty,
			ContextSize:   cfg.LLM.ContextSize,
			BaseUrl:       cfg.LLM.BaseURL,
			Token:         cfg.LLM.Token,
			Timeout:       2 * time.Minute,
		},
		WebServer:  webServer,
		StageNames: stageNames,
		Debug:      debug,
	}

	// 5. Wire things together (Server handles UI, Processor handles Analysis)
	SetupServerHandlers(webServer, processor)

	// 6. Start the Main Processing Loop
	processor.Run()
}
