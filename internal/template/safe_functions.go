package template

import (
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

// SafeFunctionRegistry provides allowlisted Sprig functions
type SafeFunctionRegistry struct {
	functions template.FuncMap
}

// NewSafeFunctionRegistry creates a registry with allowlisted Sprig functions
func NewSafeFunctionRegistry() *SafeFunctionRegistry {
	return &SafeFunctionRegistry{
		functions: createSafeFunctionMap(),
	}
}

// GetFunctions returns the allowlisted function map
func (r *SafeFunctionRegistry) GetFunctions() template.FuncMap {
	return r.functions
}

// createSafeFunctionMap builds the allowlisted function map from Sprig
func createSafeFunctionMap() template.FuncMap {
	// Get all Sprig functions and filter to safe subset
	allFunctions := sprig.TxtFuncMap()
	safeFunctions := template.FuncMap{}

	// Helper function to safely add functions
	addIfExists := func(safeName, sprigName string) {
		if fn, exists := allFunctions[sprigName]; exists {
			safeFunctions[safeName] = fn
		}
	}

	// String manipulation functions
	addIfExists("trim", "trim")
	addIfExists("trimSpace", "trimSpace")
	addIfExists("title", "title")
	addIfExists("upper", "upper")
	addIfExists("lower", "lower")
	addIfExists("replace", "replace")
	addIfExists("split", "split")
	addIfExists("join", "join")

	// Encoding functions
	addIfExists("b64enc", "b64enc")
	addIfExists("b64dec", "b64dec")
	addIfExists("base64encode", "b64enc")
	addIfExists("base64decode", "b64dec")
	addIfExists("urlquery", "urlquery")

	// Math functions
	addIfExists("add", "add")
	addIfExists("sub", "sub")
	addIfExists("mul", "mul")
	addIfExists("div", "div")
	addIfExists("mod", "mod")
	addIfExists("max", "max")
	addIfExists("min", "min")

	// Logic functions
	addIfExists("default", "default")
	addIfExists("empty", "empty")
	addIfExists("coalesce", "coalesce")
	addIfExists("ternary", "ternary")

	// Date functions (safe subset)
	addIfExists("now", "now")
	addIfExists("date", "date")

	return safeFunctions
}

// IsAllowedFunction checks if a function name is in the allowlist
func (r *SafeFunctionRegistry) IsAllowedFunction(name string) bool {
	_, exists := r.functions[name]
	return exists
}

// GetAllowedFunctionNames returns a list of all allowed function names
func (r *SafeFunctionRegistry) GetAllowedFunctionNames() []string {
	names := make([]string, 0, len(r.functions))
	for name := range r.functions {
		names = append(names, name)
	}
	return names
}
