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

	// String manipulation functions
	safeFunctions["trim"] = allFunctions["trim"]
	safeFunctions["trimSpace"] = allFunctions["trimSpace"]
	safeFunctions["title"] = allFunctions["title"]
	safeFunctions["upper"] = allFunctions["upper"]
	safeFunctions["lower"] = allFunctions["lower"]
	safeFunctions["replace"] = allFunctions["replace"]
	safeFunctions["split"] = allFunctions["split"]
	safeFunctions["join"] = allFunctions["join"]

	// Encoding functions
	safeFunctions["b64enc"] = allFunctions["b64enc"]
	safeFunctions["b64dec"] = allFunctions["b64dec"]
	safeFunctions["base64encode"] = allFunctions["b64enc"]
	safeFunctions["base64decode"] = allFunctions["b64dec"]
	safeFunctions["urlquery"] = allFunctions["urlquery"]

	// Math functions
	safeFunctions["add"] = allFunctions["add"]
	safeFunctions["sub"] = allFunctions["sub"]
	safeFunctions["mul"] = allFunctions["mul"]
	safeFunctions["div"] = allFunctions["div"]
	safeFunctions["mod"] = allFunctions["mod"]
	safeFunctions["max"] = allFunctions["max"]
	safeFunctions["min"] = allFunctions["min"]

	// Logic functions
	safeFunctions["default"] = allFunctions["default"]
	safeFunctions["empty"] = allFunctions["empty"]
	safeFunctions["coalesce"] = allFunctions["coalesce"]
	safeFunctions["ternary"] = allFunctions["ternary"]

	// Date functions (safe subset)
	safeFunctions["now"] = allFunctions["now"]
	safeFunctions["date"] = allFunctions["date"]

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
