package setup

import (
	"fmt"
	"strings"
)

type ModelGroup struct {
	Family string
	Blurb  string
	Models []string
}

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

func RecommendedModelIDs() []string {
	var out []string
	for _, g := range RecommendedModels {
		out = append(out, g.Models...)
	}
	return out
}

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
