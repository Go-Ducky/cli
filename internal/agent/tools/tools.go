package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Go-Ducky/cli/internal/provider"
)

type Context struct {
	WorkDir string

	Approval func(description string, args map[string]any) bool

	OnLog func(msg string)
}

type Result struct {
	Content string
	IsError bool
}

type Tool interface {
	Name() string
	Description() string

	Parameters() json.RawMessage

	Execute(ctx context.Context, tctx *Context, args json.RawMessage) (*Result, error)

	ToProvider() provider.Tool
}

type base struct {
	name        string
	description string
	params      json.RawMessage
}

func (b *base) Name() string                { return b.name }
func (b *base) Description() string         { return b.description }
func (b *base) Parameters() json.RawMessage { return b.params }

func (b *base) ToProvider() provider.Tool {
	return provider.Tool{
		Name:        b.name,
		Description: b.description,
		Parameters:  b.params,
	}
}

type Registry struct {
	tools map[string]Tool
	order []string
}

func NewRegistry(ts ...Tool) *Registry {
	r := &Registry{tools: map[string]Tool{}}
	for _, t := range ts {
		r.tools[t.Name()] = t
		r.order = append(r.order, t.Name())
	}
	return r
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

func (r *Registry) Size() int { return len(r.order) }

func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.tools[n])
	}
	return out
}

func (r *Registry) ToProviderTools() []provider.Tool {
	ts := r.All()
	out := make([]provider.Tool, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ToProvider())
	}
	return out
}

func ParseArgs[T any](raw json.RawMessage, target *T) error {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func DefaultRegistry() *Registry {
	return NewRegistry(
		NewRead(),
		NewWrite(),
		NewEdit(),
		NewBash(),
		NewList(),
		NewGlob(),
		NewGrep(),
	)
}
