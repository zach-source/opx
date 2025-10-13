package template

import (
	"testing"
)

func TestNewSafeFunctionRegistry(t *testing.T) {
	registry := NewSafeFunctionRegistry()
	if registry == nil {
		t.Fatal("NewSafeFunctionRegistry returned nil")
	}

	functions := registry.GetFunctions()
	if functions == nil {
		t.Fatal("GetFunctions returned nil")
	}

	if len(functions) == 0 {
		t.Error("Expected non-empty function map")
	}
}

func TestSafeFunctionAllowlist(t *testing.T) {
	registry := NewSafeFunctionRegistry()

	// Test that safe functions are present
	safeFunctions := []string{
		"base64encode", "base64decode",
		"trim", "title", "upper", "lower", "replace", "split", "join",
		"add", "sub", "mul", "div", "mod", "max", "min",
		"default", "empty", "coalesce", "ternary",
		"now", "date",
	}

	for _, fn := range safeFunctions {
		if !registry.IsAllowedFunction(fn) {
			t.Errorf("Safe function %s not found in allowlist", fn)
		}
	}
}

func TestBlockedFunctions(t *testing.T) {
	registry := NewSafeFunctionRegistry()

	// Test that dangerous functions are blocked
	blockedFunctions := []string{
		"env", "expandenv", "exec",
		"readFile", "writeFile", "glob",
		"getHostByName", "httpGet",
		"genPrivateKey", "genCert", "bcrypt",
	}

	for _, fn := range blockedFunctions {
		if registry.IsAllowedFunction(fn) {
			t.Errorf("Dangerous function %s found in allowlist (should be blocked)", fn)
		}
	}
}

func TestGetAllowedFunctionNames(t *testing.T) {
	registry := NewSafeFunctionRegistry()
	names := registry.GetAllowedFunctionNames()

	if len(names) == 0 {
		t.Error("Expected non-empty function names list")
	}

	// Verify all returned names are actually allowed
	for _, name := range names {
		if !registry.IsAllowedFunction(name) {
			t.Errorf("Function %s returned by GetAllowedFunctionNames but not allowed", name)
		}
	}

	// Test specific functions we know should be present
	expectedFunctions := []string{"base64encode", "default", "upper", "trim"}
	for _, expected := range expectedFunctions {
		found := false
		for _, name := range names {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected function %s not found in allowed function names", expected)
		}
	}
}
