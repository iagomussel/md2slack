package renderer

import (
	"fmt"
	"md2slack/internal/gitdiff"
	"strings"
)

func RenderReport(date string, groups []gitdiff.GroupedTask, allTasks []gitdiff.TaskChange, nextActions []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("```\nDaily Status Report %s \n```\n\n", date))

	var manualTasks []gitdiff.TaskChange
	var commitTasks []gitdiff.TaskChange

	for _, task := range allTasks {
		if task.IsManual {
			manualTasks = append(manualTasks, task)
		} else {
			commitTasks = append(commitTasks, task)
		}
	}
	sb.WriteString("**Tasks**\n")

	if len(manualTasks) > 0 {
		for _, task := range manualTasks {
			sb.WriteString(renderTask(task))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(commitTasks) > 0 {
		for _, task := range commitTasks {
			sb.WriteString(renderTask(task))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n**Any Blockers?**\nNo\n\n")
	sb.WriteString("**What do you plan to do next?**\n")
	if len(nextActions) == 0 {
		sb.WriteString("- Continue ongoing deliveries\n")
	} else {
		for _, action := range nextActions {
			sb.WriteString(fmt.Sprintf("- %s\n", action))
		}
	}

	return sb.String()
}

func renderTask(t gitdiff.TaskChange) string {
	hours := 1
	if t.EstimatedHours > 0 {
		hours = int(t.EstimatedHours)
	}

	intent := t.TaskIntent
	if len(intent) > 0 {
		intent = strings.ToUpper(intent[:1]) + intent[1:]
	}

	status := strings.ToLower(strings.TrimSpace(t.Status))
	statusLabel := "Done"
	statusIcon := "✅"
	switch status {
	case "inprogress", "in_progress":
		statusLabel = "In progress"
		statusIcon = "🕒"
	case "onhold", "on_hold":
		statusLabel = "On hold"
		statusIcon = "⏸"
	case "done", "":
		statusLabel = "Done"
		statusIcon = "✅"
	}

	var details []string
	for _, line := range strings.Split(t.TechnicalWhy, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		details = append(details, fmt.Sprintf("  - %s", line))
	}
	var detailsStr string
	if len(details) > 0 {
		detailsStr = strings.Join(details, "\n") + "\n"
	}

	commitsLine := ""
	if len(t.Commits) > 0 {
		commitsLine = fmt.Sprintf("\n  - commits: `%s`", strings.Join(t.Commits, "`, `"))
	}

	return fmt.Sprintf("- %s — **%dh %s** %s\n%s%s",
		intent,
		hours,
		statusLabel,
		statusIcon,
		detailsStr,
		commitsLine,
	)
}
