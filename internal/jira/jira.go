// Package jira fetches issues from Jira Cloud (REST API v3) for daily status reports.
package jira

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"md2slack/internal/config"
	"md2slack/internal/gitdiff"
)

const searchPath = "/rest/api/3/search"

// DailyIssues returns issues touched on the given calendar day (site timezone)
// in the configured project, mapped to TaskChange for the existing report pipeline.
func DailyIssues(cfg *config.JiraConfig, reportDate string) ([]gitdiff.TaskChange, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	iso, err := normalizeToISODate(reportDate)
	if err != nil {
		return nil, err
	}

	jql := strings.TrimSpace(cfg.JQL)
	if jql != "" {
		jql = strings.ReplaceAll(jql, "{date}", iso)
	} else {
		pk := strings.TrimSpace(cfg.ProjectKey)
		jql = fmt.Sprintf(`project = %s AND updatedDate = "%s"`, pk, iso)
		if cfg.AssigneeIsMe {
			jql += ` AND assignee = currentUser()`
		}
	}

	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	all, err := searchAll(base, cfg.Email, cfg.APIToken, jql)
	if err != nil {
		return nil, err
	}

	repoKey := config.JiraStorageKey(cfg)
	out := make([]gitdiff.TaskChange, 0, len(all))
	for _, iss := range all {
		out = append(out, issueToTask(iss, repoKey, reportDate, base))
	}
	return out, nil
}

func normalizeToISODate(reportDate string) (string, error) {
	s := strings.TrimSpace(reportDate)
	if s == "" {
		return "", fmt.Errorf("empty date")
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("2006-01-02"), nil
	}
	if t, err := time.Parse("01-02-2006", s); err == nil {
		return t.Format("2006-01-02"), nil
	}
	return "", fmt.Errorf("date must be YYYY-MM-DD or MM-DD-YYYY: %q", reportDate)
}

type searchResponse struct {
	Issues []issue `json:"issues"`
	Total  int     `json:"total"`
}

type issue struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Fields issueFields `json:"fields"`
}

type issueFields struct {
	Summary  string `json:"summary"`
	Updated  string `json:"updated"`
	Status   status `json:"status"`
	Assignee *struct {
		DisplayName string `json:"displayName"`
	} `json:"assignee"`
	AggregateTimeSpentSeconds int `json:"aggregatetimespent"`
}

type status struct {
	Name           string         `json:"name"`
	StatusCategory statusCategory `json:"statusCategory"`
}

type statusCategory struct {
	Key string `json:"key"`
}

func searchAll(base, email, apiToken, jql string) ([]issue, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	var combined []issue
	start := 0
	page := 50

	for {
		body, err := json.Marshal(map[string]interface{}{
			"jql":        jql,
			"startAt":    start,
			"maxResults": page,
			"fields": []string{
				"summary", "status", "assignee", "aggregatetimespent", "updated",
			},
		})
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest(http.MethodPost, base+searchPath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		auth := base64.StdEncoding.EncodeToString([]byte(email + ":" + apiToken))
		req.Header.Set("Authorization", "Basic "+auth)

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("jira search %s: %s — %s", resp.Status, jql, strings.TrimSpace(string(data)))
		}

		var parsed searchResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("jira response: %w", err)
		}
		combined = append(combined, parsed.Issues...)
		if len(parsed.Issues) < page || len(combined) >= parsed.Total {
			break
		}
		start += page
	}
	return combined, nil
}

func issueToTask(iss issue, repoName, date, baseURL string) gitdiff.TaskChange {
	hours := 1
	if iss.Fields.AggregateTimeSpentSeconds > 0 {
		h := iss.Fields.AggregateTimeSpentSeconds / 3600
		if h < 1 {
			h = 1
		}
		hours = h
	}

	st := strings.ToLower(strings.TrimSpace(iss.Fields.Status.StatusCategory.Key))
	statusVal := "inprogress"
	switch st {
	case "done":
		statusVal = "done"
	case "new":
		statusVal = "todo"
	default:
		statusVal = "inprogress"
	}

	details := fmt.Sprintf("Jira: %s", iss.Fields.Status.Name)
	if iss.Fields.Assignee != nil && iss.Fields.Assignee.DisplayName != "" {
		details += fmt.Sprintf(" — %s", iss.Fields.Assignee.DisplayName)
	}
	link := fmt.Sprintf("%s/browse/%s", strings.TrimRight(baseURL, "/"), iss.Key)
	details += fmt.Sprintf("\n%s", link)

	intent := fmt.Sprintf("%s: %s", iss.Key, iss.Fields.Summary)

	return gitdiff.TaskChange{
		ID:             iss.ID,
		RepoName:       repoName,
		Date:           date,
		TaskIntent:     intent,
		Intent:         intent,
		Commits:        []string{iss.Key},
		EstimatedHours: float64(hours),
		Details:        details,
		Status:         statusVal,
		Scope:          "jira",
		TaskType:       "jira",
		IsManual:       false,
	}
}
