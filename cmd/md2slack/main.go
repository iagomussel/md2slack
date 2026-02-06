package main

import (
	"flag"
	"fmt"
	"md2slack/internal/config"
	"md2slack/internal/llm"
	"md2slack/internal/webui"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() {
	var debug bool
	var install bool
	var web bool
	var webAddr string
	flag.BoolVar(&debug, "debug", false, "Enable debug mode")
	flag.BoolVar(&install, "install", false, "Install the binary")
	flag.BoolVar(&web, "web", false, "Enable web UI")
	flag.StringVar(&webAddr, "web-addr", "127.0.0.1:8080", "Web UI address")
	flag.Parse()

	if install {
		runInstall()
		return
	}

	args := flag.Args()
	if len(args) < 1 && !web {
		fmt.Println("usage: ssbot [--debug] [--web] [--web-addr host:port] [--install] [<MM-DD-YYYY>] [extra context]")
		os.Exit(1)
	}

	var dates []string
	extra := ""
	if len(args) > 0 {
		dates = []string{args[0]}
		if strings.Contains(args[0], ",") {
			dates = strings.Split(args[0], ",")
		}
		if len(args) > 1 {
			extra = strings.Join(args[1:], " ")
			// Clean terminal artifacts like bracketed paste markers
			re := regexp.MustCompile(`(?i)\x1b\[\d+~`)
			extra = re.ReplaceAllString(extra, "")
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	flagWebAddr := flag.Lookup("web-addr")
	webAddrDefault := "127.0.0.1:8080"
	if flagWebAddr != nil {
		if flagWebAddr.DefValue != "" {
			webAddrDefault = flagWebAddr.DefValue
		}
	}

	if webAddr == webAddrDefault {
		webAddr = net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
		if web {
			resolved, err := resolveWebAddr(cfg.Server.Host, cfg.Server.Port, cfg.Server.AutoIncrementPort)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error resolving web address: %v\n", err)
				os.Exit(1)
			}
			webAddr = resolved
		}
	}

	stageNames := []string{
		"Preparing commit context",
		"Summarizing commits",
		"Generating tasks",
		"Reviewing tasks",
		"Suggesting next actions",
		"Rendering report",
	}

	var webServer *webui.Server
	if web {
		webServer = webui.Start(webAddr, stageNames)
	}

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
			Debug:         debug,
			Timeout:       2 * time.Minute,
			Heartbeat:     5 * time.Second,
		},
		WebServer:  webServer,
		Debug:      debug,
		StageNames: stageNames,
	}

	if len(dates) == 0 && webServer != nil {
		for req := range webServer.RunChannel() {
			processor.ProcessDate(req.Date, req.RepoPath, req.Author, "")
		}
		return
	}

	for _, date := range dates {
		processor.ProcessDate(date, "", "", extra)
	}
}
