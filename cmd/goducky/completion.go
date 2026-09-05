package main

import (
	"fmt"
	"strings"
)

const bashCompletion = `_goducky() {
    local cur prev
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    local flags="--version --models --yes --provider --model --base-url --key --login --dir -p --help"
    local commands="completion update mcp sessions resume rename"
    local providers="ollama groq openai openai_compatible anthropic gemini openrouter"
    if [[ ${COMP_CWORD} -eq 1 && "${cur}" != -* ]]; then
        COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
        return 0
    fi
    case "${prev}" in
        --provider|--login) COMPREPLY=( $(compgen -W "${providers}" -- "${cur}") ); return 0 ;;
    esac
    if [[ "${cur}" == -* ]]; then
        COMPREPLY=( $(compgen -W "${flags}" -- "${cur}") )
        return 0
    fi
    COMPREPLY=( $(compgen -f -- "${cur}") )
}
complete -F _goducky goducky
`

const zshCompletion = `#compdef goducky
_goducky() {
    _arguments \
        '1:command:(completion update mcp sessions resume rename)' \
        '(-p --p)'{-p,--p}'[run a one-shot prompt and exit]:prompt: ' \
        '--provider[AI provider]:provider:(ollama groq openai openai_compatible anthropic gemini openrouter)' \
        '--model[model name]:model: ' \
        '--base-url[base URL for OpenAI-compatible endpoints]:url: ' \
        '--key[API key]:key: ' \
        '--login[save an API key for a provider]:provider:(groq openai anthropic gemini openrouter)' \
        '--models[list available models]' \
        '--yes[auto-approve all tool actions]' \
        '--dir[working directory]:directory:_directories' \
        '--version[print version]' \
        '--help[show help]'
}
_goducky "$@"
`

const fishCompletion = `complete -c goducky -f
complete -c goducky -n '__fish_use_subcommand' -a update -d 'self-update to the latest release'
complete -c goducky -n '__fish_use_subcommand' -a completion -d 'generate a shell completion script'
complete -c goducky -n '__fish_use_subcommand' -a mcp -d 'run an MCP stdio server for the current directory'
complete -c goducky -n '__fish_use_subcommand' -a sessions -d 'list saved chats'
complete -c goducky -n '__fish_use_subcommand' -a resume -d 'resume a saved chat'
complete -c goducky -n '__fish_use_subcommand' -a rename -d 'rename a saved chat'
complete -c goducky -l p -r -d 'run a one-shot prompt and exit'
complete -c goducky -l provider -r -a 'ollama groq openai openai_compatible anthropic gemini openrouter' -d 'AI provider'
complete -c goducky -l model -r -d 'model name (overrides config)'
complete -c goducky -l base-url -r -d 'base URL for OpenAI-compatible endpoints'
complete -c goducky -l key -r -d 'API key (overrides config/env)'
complete -c goducky -l login -r -a 'groq openai anthropic gemini openrouter' -d 'save an API key for a provider'
complete -c goducky -l dir -r -d 'working directory (default: current)'
complete -c goducky -l models -d 'list available models and exit'
complete -c goducky -l yes -d 'auto-approve all tool actions'
complete -c goducky -l version -d 'print version and exit'
complete -c goducky -l help -d 'show help'
`

const powershellCompletion = `Register-ArgumentCompleter -Native -CommandName goducky -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $flags = @('--version', '--models', '--yes', '--provider', '--model', '--base-url', '--key', '--login', '--dir', '-p', '--help')
    $commands = @('update', 'completion', 'mcp', 'sessions', 'resume', 'rename')
    $providers = @('ollama', 'groq', 'openai', 'openai_compatible', 'anthropic', 'gemini', 'openrouter')
    $completions = @()
    if ($commandAst.CommandElements.Count -gt 1) {
        $prev = $commandAst.CommandElements[$commandAst.CommandElements.Count - 1].Text
        if ($prev -eq '--provider' -or $prev -eq '--login') { $completions = $providers }
        else { $completions = $flags }
    } else {
        $completions = $flags + $commands
    }
    $completions | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`

func completionCmd(args []string) error {
	shell := "bash"
	if len(args) > 0 {
		shell = strings.ToLower(strings.TrimSpace(args[0]))
	}
	var script string
	switch shell {
	case "bash":
		script = bashCompletion
	case "zsh":
		script = zshCompletion
	case "fish":
		script = fishCompletion
	case "powershell", "ps1":
		script = powershellCompletion
	default:
		return fmt.Errorf("unknown shell %q (supported: bash, zsh, fish, powershell)", shell)
	}
	fmt.Print(script)
	return nil
}
