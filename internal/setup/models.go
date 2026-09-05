package setup

import (
	"fmt"
	"strings"
)

// ModelGroup groups recommended local models by family for the picker.
type ModelGroup struct {
	Family string
	Blurb  string
	Models []string
}

// RecommendedModels is the curated shortlist shown after installing Ollama,
// ordered largest to smallest within each family so the user can trade speed
// for quality.
var RecommendedModels = []ModelGroup{
	{
		Family: "Qwen",
		Blurb:  "Best overall coding ability",
		Models: []string{"qwen3-coder:30b", "qwen2.5-coder:14b", "qwen2.5-coder:7b", "qwen2.5-coder:3b"},
	},
	{
		Family: "Starcoder",
		Blurb:  "Efficient code completion",
		Models: []string{"starcoder2:15b", "starcoder2:7b", "starcoder2:3b"},
	},
	{
		Family: "Deepseek",
		Blurb:  "Reasoning-oriented coder/chat",
		Models: []string{"deepseek-r1:14b", "deepseek-r1:8b", "deepseek-r1:7b", "deepseek-r1:1.5b"},
	},
	{
		Family: "Codegemma",
		Blurb:  "Google code models",
		Models: []string{"codegemma:7b", "codegemma:2b"},
	},
	{
		Family: "Llama",
		Blurb:  "General and chatty",
		Models: []string{"llama3.2:3b", "llama3.2:1b"},
	},
}

// RecommendedModelIDs flattens the recommended shortlist into plain model ids.
func RecommendedModelIDs() []string {
	var out []string
	for _, g := range RecommendedModels {
		out = append(out, g.Models...)
	}
	return out
}

// RecommendedModelOptions flattens the groups into picker labels like
// "Qwen · qwen2.5-coder:7b", followed by "Skip, go straight to chat" and
// "Quit (exit)".
func RecommendedModelOptions() []string {
	var out []string
	for _, g := range RecommendedModels {
		for _, m := range g.Models {
			out = append(out, fmt.Sprintf("%s · %s", g.Family, m))
		}
	}
	out = append(out, "── Skip, go straight to chat ──", "── Quit (exit) ──")
	return out
}

// ModelFromOption extracts the model id from a picker option label.
// It returns the empty string for the skip/quit actions.
func ModelFromOption(opt string) string {
	if strings.Contains(opt, "──") {
		return ""
	}
	_, after, ok := strings.Cut(opt, "·")
	if !ok {
		return strings.TrimSpace(opt)
	}
	return strings.TrimSpace(after)
}
