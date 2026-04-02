package llm

import (
	"fmt"
	"sync"
)

// Fabric is a model router that manages multiple LLM providers and selects
// the appropriate one based on the requested model name.
// Pattern: Factory + Registry.
type Fabric struct {
	mu        sync.RWMutex
	providers map[string]Provider   // "gemini" -> GeminiProvider
	models    map[string]string     // "gemini-2.5-pro" -> "gemini", "grok-3" -> "grok"
	fallback  string                // default provider name
}

// NewFabric creates a new model fabric with the given default provider.
func NewFabric(defaultProvider string) *Fabric {
	return &Fabric{
		providers: make(map[string]Provider),
		models:    make(map[string]string),
		fallback:  defaultProvider,
	}
}

// RegisterProvider adds a provider to the fabric.
func (f *Fabric) RegisterProvider(provider Provider) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.providers[provider.Name()] = provider
}

// RegisterModel maps a model name to a provider.
// Example: RegisterModel("gemini-2.5-pro", "gemini")
func (f *Fabric) RegisterModel(modelName, providerName string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.models[modelName] = providerName
}

// GetProvider returns the provider for a given model name.
// If the model is unknown, it falls back to the default provider.
func (f *Fabric) GetProvider(modelName string) (Provider, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Try to resolve by model name
	if providerName, ok := f.models[modelName]; ok {
		if p, exists := f.providers[providerName]; exists {
			return p, nil
		}
	}

	// Try direct provider name match
	if p, exists := f.providers[modelName]; exists {
		return p, nil
	}

	// Fallback to default
	if p, exists := f.providers[f.fallback]; exists {
		return p, nil
	}

	return nil, fmt.Errorf("no provider found for model %q and no fallback available", modelName)
}

// ListProviders returns the names of all registered providers.
func (f *Fabric) ListProviders() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	names := make([]string, 0, len(f.providers))
	for name := range f.providers {
		names = append(names, name)
	}
	return names
}

// ListModels returns all registered model-to-provider mappings.
func (f *Fabric) ListModels() map[string]string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make(map[string]string, len(f.models))
	for k, v := range f.models {
		result[k] = v
	}
	return result
}
