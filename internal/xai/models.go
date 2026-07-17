// Package xai provides xAI OAuth authentication and chat completions.
package xai

import "slices"

// Effort levels for models that support reasoning_effort.
const (
	EffortLow    = "low"
	EffortMedium = "medium"
	EffortHigh   = "high"
)

// Model describes a selectable xAI model and whether effort applies.
type Model struct {
	ID             string
	Name           string
	Description    string
	SupportsEffort bool
	// EffortOptions lists allowed efforts when SupportsEffort is true.
	EffortOptions []string
	// DefaultEffort used when SupportsEffort and none chosen.
	DefaultEffort string
}

// Catalog is the built-in model list (data-driven for menus).
var Catalog = []Model{
	{
		ID:             "grok-4.5",
		Name:           "Grok 4.5",
		Description:    "Flagship model; configurable reasoning (default high)",
		SupportsEffort: true,
		EffortOptions:  []string{EffortLow, EffortMedium, EffortHigh},
		DefaultEffort:  EffortHigh,
	},
	{
		ID:             "grok-4.3",
		Name:           "Grok 4.3",
		Description:    "General-purpose; optional reasoning effort",
		SupportsEffort: true,
		EffortOptions:  []string{EffortLow, EffortMedium, EffortHigh},
		DefaultEffort:  EffortLow,
	},
	{
		ID:             "grok-4.20-0309-reasoning",
		Name:           "Grok 4.20 Reasoning",
		Description:    "Reasoning variant (effort not configurable)",
		SupportsEffort: false,
	},
	{
		ID:             "grok-4.20-0309-non-reasoning",
		Name:           "Grok 4.20 Non-reasoning",
		Description:    "Fast non-reasoning variant",
		SupportsEffort: false,
	},
	{
		ID:             "grok-build-0.1",
		Name:           "Grok Build 0.1",
		Description:    "Coding-oriented model",
		SupportsEffort: false,
	},
}

// LookupModel returns a model by id, or false if unknown.
func LookupModel(id string) (Model, bool) {
	for _, m := range Catalog {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// EffortRequired reports whether the model needs an effort selection.
func EffortRequired(modelID string) bool {
	m, ok := LookupModel(modelID)
	return ok && m.SupportsEffort
}

// ValidEffort reports whether effort is allowed for modelID.
// Empty effort is valid when the model does not support effort.
func ValidEffort(modelID, effort string) bool {
	m, ok := LookupModel(modelID)
	if !ok {
		// Unknown models: allow empty effort only.
		return effort == ""
	}
	if !m.SupportsEffort {
		return effort == ""
	}
	if effort == "" {
		return true // default will apply at request time
	}
	return slices.Contains(m.EffortOptions, effort)
}

// ResolveEffort returns the effort to send (empty means omit from request).
func ResolveEffort(modelID, effort string) string {
	m, ok := LookupModel(modelID)
	if !ok || !m.SupportsEffort {
		return ""
	}
	if effort == "" {
		return m.DefaultEffort
	}
	if slices.Contains(m.EffortOptions, effort) {
		return effort
	}
	return m.DefaultEffort
}
