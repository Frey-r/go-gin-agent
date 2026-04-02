package tools

import (
	"context"
	"fmt"
	"sync"
)

// Executor is the interface for a tool that can be invoked by the orchestrator.
type Executor interface {
	// Execute runs the tool with the given JSON arguments and returns a result string.
	Execute(ctx context.Context, arguments string) (string, error)
}

// Registry manages available tools and dispatches execution requests.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Executor
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Executor),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(name string, executor Executor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = executor
}

// Execute dispatches a tool call by name.
func (r *Registry) Execute(ctx context.Context, name, arguments string) (string, error) {
	r.mu.RLock()
	executor, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("tool %q not found in registry", name)
	}

	return executor.Execute(ctx, arguments)
}

// List returns the names of all registered tools.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}
