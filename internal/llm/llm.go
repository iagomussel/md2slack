package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

type LLMOptions struct {
	Temperature   float64
	TopP          float64
	RepeatPenalty float64
	ModelName     string
	BaseUrl       string
}

type OllamaRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	System  string `json:"system"`
	Stream  bool   `json:"stream"`
	Options map[string]interface{}
}

type OllamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func GenerateStatusReport(context string, options LLMOptions) (string, error) {
	if len(strings.TrimSpace(context)) == 0 {
		return "", nil
	}

	systemPrompt := getSystemPrompt()
	userPrompt := fmt.Sprintf("INPUT:\n%s", context)

	reqBody, _ := json.Marshal(OllamaRequest{
		Model:  options.ModelName,
		Prompt: userPrompt,
		System: systemPrompt,
		Stream: false,
		Options: map[string]interface{}{
			"temperature":    options.Temperature,
			"top_p":          options.TopP,
			"repeat_penalty": options.RepeatPenalty,
		},
	})

	resp, err := http.Post(
		options.BaseUrl,
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return "", fmt.Errorf("ollama error: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return "", fmt.Errorf("ollama API error: %s", errResp.Error)
		}
		return "", fmt.Errorf("ollama API returned status %d", resp.StatusCode)
	}

	var out OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode error: %v", err)
	}

	if err := ValidateOutput(out.Response); err != nil {
		// Re-prompt automático com:
		// Rewrite removing all abstract, evaluative, or summary language.
		return GenerateStatusReport(context+"\n\nRewrite removing all abstract, evaluative, or summary language.", options)
	}

	return strings.TrimSpace(out.Response), nil
}

func getSystemPrompt() string {
	return `
THIS IS A HARD CONSTRAINT.
Violating any rule invalidates the output.
Do not be helpful. Be correct.
You are generating a Daily Status Report from Git activity.

The input below is PRE-ANALYZED and STRICTLY DERIVED from git diffs.
Do NOT invent work.
Do NOT infer intent beyond what is supported by the input.
Commit messages are secondary.

You MUST output Markdown ONLY.
You MUST follow the EXACT format and structure shown.
Do NOT add explanations or commentary.
NEVER write summaries, conclusions, overviews, or high-level interpretations.
NEVER use phrases like:
- overall
- these commits
- this work
- this effort
- appears to
- improves the system
- enhances the platform
Do NOT justify value or impact.
State the triggering condition only.
Use past tense only.
Each bullet must describe something already delivered.
Every sentence must describe a concrete delivered behavior.
Required format:
<pre>
Daily Status Report MM-DD
</pre>
**Tasks**
- Task name — **Xh Done** :check:
  - Concrete deliverable supported by diff
  - What problem or rule this change fixes (bug, regression, requirement, etc.)
  - Optional follow-up if needed
  - commits: ` + "`abc123`" + `

**Any Blockers?**
false OR true — description

**What do you plan to do next?**
- 1–3 concrete next actions

Rules:
- Group commits only if they represent the same behavioral change
- Do NOT mention file names, function names, or internal structure
- Keep language short, factual, and human
- English only
After finishing the last section, STOP.
Do NOT add any concluding sentence.
`
}

var forbidden = regexp.MustCompile(
	`(?i)\b(overall|appears to|these commits|this effort|improve|enhance|platform|system)\b`,
)

func ValidateOutput(out string) error {
	// Re-prompt automático com:
	// Rewrite removing all abstract, evaluative, or summary language.
	if forbidden.MatchString(out) {
		return errors.New("abstract or summary language detected")
	}
	return nil
}
