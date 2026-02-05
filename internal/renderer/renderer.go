package renderer

import (
	"fmt"
	"md2slack/internal/gitdiff"
	"strings"
)

func RenderReport(date string, groups []gitdiff.GroupedTask, allTasks []gitdiff.TaskChange) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Daily Status Report %s\n\n", date))
	sb.WriteString("**Tasks**\n")

	// If no groups, render all tasks sequentially
	if len(groups) == 0 {
		for _, task := range allTasks {
			sb.WriteString(renderTask(task))
			sb.WriteString("\n")
		}
	} else {
		for _, group := range groups {
			if group.Epic != "" && group.Epic != "none" {
				sb.WriteString(fmt.Sprintf("\n*Epic: %s*\n", group.Epic))
			}

			for _, taskIdx := range group.Tasks {
				if taskIdx < 0 || taskIdx >= len(allTasks) {
					continue
				}
				task := allTasks[taskIdx]
				sb.WriteString(renderTask(task))
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("\n**Any Blockers?**\nNo\n\n")
	sb.WriteString("**What do you plan to do next?**\n- Continue ongoing deliveries\n")

	return sb.String()
}

func renderTask(t gitdiff.TaskChange) string {
	hours := 1
	if t.EstimatedHours != nil && *t.EstimatedHours > 0 {
		hours = *t.EstimatedHours
	}

	intent := t.TaskIntent
	if len(intent) > 0 {
		intent = strings.ToUpper(intent[:1]) + intent[1:]
	}

	return fmt.Sprintf("- %s — **%dh Done** :check:\n  - %s\n  - commits: `%s`",
		intent,
		hours,
		t.TechnicalWhy,
		strings.Join(t.Commits, "`, `"),
	)
}
