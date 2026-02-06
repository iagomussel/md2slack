# ssbot - Daily Status Bot

A web-based tool for generating daily status reports from git commits using LLM analysis.

## Quick Start

```bash
# Start the web UI (default: http://127.0.0.1:8080)
./ssbot

# Use a custom port
./ssbot --web-addr localhost:3000

# Enable debug mode
./ssbot --debug

# Install to system path
./ssbot --install
```

## Features

- 🌐 **Web UI**: Modern, interactive interface for managing tasks and reports
- 🤖 **LLM-Powered**: Automatic task extraction and summarization from git commits
- 💬 **AI Assistant**: Chat interface for refining tasks
- 🔧 **Manual Editing**: Edit tasks directly in the UI
- 💾 **Persistence**: SQLite database for task history
- 🔀 **Task Management**: Merge, split, and organize tasks with AI tools

## Configuration

Edit `~/.md2slack/config.ini` to configure:
- LLM provider (Ollama, OpenAI, Anthropic)
- Model settings
- Slack integration
- Server settings

## Usage

1. **Start the server**: `./ssbot`
2. **Open browser**: Navigate to the displayed URL
3. **Select project**: Choose a git repository from the dropdown
4. **Pick date**: Select the date for your report
5. **Run Analysis**: Click "Run Analysis" to generate tasks from commits
6. **Refine**: Use the AI assistant or manual editing to refine tasks
7. **Export**: Send the report to Slack or copy as markdown

## Development

```bash
# Build the backend
go build -o ssbot ./cmd/md2slack/

# Run the frontend dev server
cd ui && npm run dev

# Build the frontend for production
cd ui && npm run build
```

## Architecture

- **Backend**: Go (webui, LLM adapters, git analysis)
- **Frontend**: SvelteKit + Vite
- **Database**: SQLite (task history)
- **LLM**: Supports Ollama, OpenAI, Anthropic via langchaingo
