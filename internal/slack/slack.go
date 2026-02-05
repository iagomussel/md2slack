package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"md2slack/internal/config"
)

type converter struct {
	source []byte
	blocks []interface{}
	attrs  map[string]interface{}
}

func SendMarkdown(cfg *config.SlackConfig, markdown string) error {
	blocks, err := ConvertToBlocks(markdown)
	if err != nil {
		return err
	}

	return sendToSlack(cfg, blocks)
}

func ConvertToBlocks(markdown string) ([]interface{}, error) {
	input := []byte(markdown)
	reader := text.NewReader(input)
	doc := goldmark.DefaultParser().Parse(reader)

	c := &converter{
		source: input,
		blocks: make([]interface{}, 0),
		attrs:  make(map[string]interface{}),
	}

	c.convert(doc)
	return c.blocks, nil
}

func sendToSlack(cfg *config.SlackConfig, blocks []interface{}) error {
	if cfg.BotToken == "YOUR_BOT_TOKEN_HERE" || cfg.ChannelID == "YOUR_CHANNEL_ID_HERE" {
		return fmt.Errorf("please configure bot_token and channel_id in config.ini")
	}

	message := map[string]interface{}{
		"channel": cfg.ChannelID,
		"blocks": []interface{}{
			map[string]interface{}{
				"type":     "rich_text",
				"elements": blocks,
			},
		},
	}

	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+cfg.BotToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var slackResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&slackResp); err != nil {
		return err
	}

	if !slackResp.OK {
		// Log the payload to stderr for debugging
		fmt.Fprintf(os.Stderr, "Slack API Error: %s\n", slackResp.Error)
		fmt.Fprintf(os.Stderr, "Payload: %s\n", string(payload))
		return fmt.Errorf("slack error: %s", slackResp.Error)
	}

	return nil
}

func (c *converter) convert(node ast.Node) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			switch n.(type) {
			case *ast.Emphasis:
				delete(c.attrs, "bold")
				delete(c.attrs, "italic")
			case *ast.Heading:
				delete(c.attrs, "bold")
			case *ast.Link:
				delete(c.attrs, "link")
			case *ast.CodeSpan:
				delete(c.attrs, "code")
			}
			return ast.WalkContinue, nil
		}

		switch t := n.(type) {
		case *ast.Heading:
			c.attrs["bold"] = true
			return ast.WalkContinue, nil
		case *ast.Emphasis:
			if t.Level == 2 {
				c.attrs["bold"] = true
			} else {
				c.attrs["italic"] = true
			}
			return ast.WalkContinue, nil
		case *ast.Link:
			c.attrs["link"] = string(t.Destination)
			return ast.WalkContinue, nil
		case *ast.CodeSpan:
			c.attrs["code"] = true
			return ast.WalkContinue, nil
		case *ast.Text:
			c.addTextToLastSection(string(t.Segment.Value(c.source)))
			if t.SoftLineBreak() || t.HardLineBreak() {
				c.addTextToLastSection("\n")
			}
			return ast.WalkContinue, nil
		case *ast.Paragraph:
			if n.Parent() != nil && n.Parent().Kind().String() == "ListItem" {
				if n.PreviousSibling() != nil {
					c.addTextToLastSection("\n")
				}
				return ast.WalkContinue, nil
			}
			c.blocks = append(c.blocks, map[string]interface{}{
				"type":     "rich_text_section",
				"elements": []interface{}{},
			})
			return ast.WalkContinue, nil
		case *ast.List:
			return ast.WalkContinue, nil
		case *ast.ListItem:
			l := n.Parent().(*ast.List)
			style := "bullet"
			if l.IsOrdered() {
				style = "ordered"
			}
			indent := 0
			p := n.Parent()
			for p != nil {
				if _, ok := p.(*ast.List); ok {
					if p.Parent() != nil && p.Parent().Kind().String() == "ListItem" {
						indent++
					}
				}
				p = p.Parent()
			}

			listBlock := c.findOrCreateList(style, indent)

			itemSection := map[string]interface{}{
				"type":     "rich_text_section",
				"elements": []interface{}{},
			}

			originalBlocks := c.blocks
			c.blocks = []interface{}{itemSection}

			child := n.FirstChild()
			for child != nil {
				if _, ok := child.(*ast.List); ok {
				} else {
					ast.Walk(child, func(cn ast.Node, entering bool) (ast.WalkStatus, error) {
						if entering {
							if _, ok := cn.(*ast.List); ok {
								return ast.WalkSkipChildren, nil
							}
						}
						return c.convertScoped(cn, entering)
					})
				}
				child = child.NextSibling()
			}

			processedItemSection := c.blocks[0]
			c.blocks = originalBlocks

			elements := listBlock["elements"].([]interface{})
			listBlock["elements"] = append(elements, processedItemSection)

			child = n.FirstChild()
			for child != nil {
				if _, ok := child.(*ast.List); ok {
					c.convert(child)
				}
				child = child.NextSibling()
			}

			return ast.WalkSkipChildren, nil

		case *ast.FencedCodeBlock, *ast.CodeBlock:
			var codeText strings.Builder
			lines := t.Lines()
			for i := 0; i < lines.Len(); i++ {
				line := lines.At(i)
				codeText.Write(line.Value(c.source))
			}
			c.blocks = append(c.blocks, map[string]interface{}{
				"type": "rich_text_preformatted",
				"elements": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": codeText.String(),
					},
				},
			})
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})
}

func (c *converter) findOrCreateList(style string, indent int) map[string]interface{} {
	if len(c.blocks) > 0 {
		last := c.blocks[len(c.blocks)-1].(map[string]interface{})
		if last["type"] == "rich_text_list" && last["style"] == style && last["indent"] == indent {
			return last
		}
	}

	newList := map[string]interface{}{
		"type":     "rich_text_list",
		"style":    style,
		"indent":   indent,
		"elements": []interface{}{},
	}
	c.blocks = append(c.blocks, newList)
	return newList
}

func (c *converter) convertScoped(n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		switch n.(type) {
		case *ast.Emphasis:
			delete(c.attrs, "bold")
			delete(c.attrs, "italic")
		case *ast.Link:
			delete(c.attrs, "link")
		case *ast.CodeSpan:
			delete(c.attrs, "code")
		}
		return ast.WalkContinue, nil
	}

	switch t := n.(type) {
	case *ast.Emphasis:
		if t.Level == 2 {
			c.attrs["bold"] = true
		} else {
			c.attrs["italic"] = true
		}
	case *ast.Link:
		c.attrs["link"] = string(t.Destination)
	case *ast.CodeSpan:
		c.attrs["code"] = true
	case *ast.Text:
		c.addTextToLastSection(string(t.Segment.Value(c.source)))
		if t.SoftLineBreak() || t.HardLineBreak() {
			c.addTextToLastSection("\n")
		}
	}
	return ast.WalkContinue, nil
}

func (c *converter) addTextToLastSection(text string) {
	if len(c.blocks) == 0 {
		c.blocks = append(c.blocks, map[string]interface{}{
			"type":     "rich_text_section",
			"elements": []interface{}{},
		})
	}

	lastBlock := c.blocks[len(c.blocks)-1].(map[string]interface{})
	if lastBlock["type"] != "rich_text_section" {
		lastBlock = map[string]interface{}{
			"type":     "rich_text_section",
			"elements": []interface{}{},
		}
		c.blocks = append(c.blocks, lastBlock)
	}

	elements := lastBlock["elements"].([]interface{})

	emojiRegex := regexp.MustCompile(`:([a-z0-9_+-]+):`)
	matches := emojiRegex.FindAllStringIndex(text, -1)
	lastIdx := 0

	for _, match := range matches {
		if match[0] > lastIdx {
			elements = append(elements, c.makeTextElement(text[lastIdx:match[0]]))
		}
		emojiName := strings.Trim(text[match[0]:match[1]], ":")
		elements = append(elements, map[string]interface{}{
			"type": "emoji",
			"name": emojiName,
		})
		lastIdx = match[1]
	}

	if lastIdx < len(text) {
		elements = append(elements, c.makeTextElement(text[lastIdx:]))
	}

	lastBlock["elements"] = elements
}

func (c *converter) makeTextElement(text string) map[string]interface{} {
	el := map[string]interface{}{
		"type": "text",
		"text": text,
	}

	style := map[string]bool{}
	if b, ok := c.attrs["bold"].(bool); ok && b {
		style["bold"] = true
	}
	if i, ok := c.attrs["italic"].(bool); ok && i {
		style["italic"] = true
	}
	if code, ok := c.attrs["code"].(bool); ok && code {
		style["code"] = true
	}

	if len(style) > 0 {
		el["style"] = style
	}

	if link, ok := c.attrs["link"].(string); ok {
		el["type"] = "link"
		el["url"] = link
	}

	return el
}
