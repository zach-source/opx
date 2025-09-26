package main

import (
	"reflect"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected parsedArgs
	}{
		{
			name: "flag before command",
			args: []string{"--account=ACC123", "read", "op://vault/item/field"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item/field"},
				opFlags: []string{"--account=ACC123"},
			},
		},
		{
			name: "flag after command",
			args: []string{"read", "op://vault/item/field", "--account=ACC123"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item/field"},
				opFlags: []string{"--account=ACC123"},
			},
		},
		{
			name: "flag with space before command",
			args: []string{"--account", "ACC123", "read", "op://vault/item/field"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item/field"},
				opFlags: []string{"--account=ACC123"},
			},
		},
		{
			name: "flag with space after command",
			args: []string{"read", "op://vault/item/field", "--account", "ACC123"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item/field"},
				opFlags: []string{"--account=ACC123"},
			},
		},
		{
			name: "multiple flags",
			args: []string{"--account=ACC1", "read", "op://vault/item/field", "--account=ACC2"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item/field"},
				opFlags: []string{"--account=ACC1", "--account=ACC2"},
			},
		},
		{
			name: "multiple refs",
			args: []string{"read", "op://vault/item1/field", "op://vault/item2/field", "--account=ACC123"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item1/field", "op://vault/item2/field"},
				opFlags: []string{"--account=ACC123"},
			},
		},
		{
			name: "no flags",
			args: []string{"read", "op://vault/item/field"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item/field"},
				opFlags: nil,
			},
		},
		{
			name: "empty account flag ignored",
			args: []string{"--account=", "read", "op://vault/item/field"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item/field"},
				opFlags: nil,
			},
		},
		{
			name: "status command no args",
			args: []string{"status"},
			expected: parsedArgs{
				command: "status",
				args:    nil,
				opFlags: nil,
			},
		},
		{
			name: "resolve command",
			args: []string{"--account=ACC123", "resolve", "DB_PASSWORD=op://vault/db/password"},
			expected: parsedArgs{
				command: "resolve",
				args:    []string{"DB_PASSWORD=op://vault/db/password"},
				opFlags: []string{"--account=ACC123"},
			},
		},
		{
			name: "flags interspersed",
			args: []string{"--account=ACC1", "read", "--account=ACC2", "op://vault/item/field", "--account=ACC3"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item/field"},
				opFlags: []string{"--account=ACC1", "--account=ACC2", "--account=ACC3"},
			},
		},
		{
			name: "empty args",
			args: []string{},
			expected: parsedArgs{
				command: "",
				args:    nil,
				opFlags: nil,
			},
		},
		{
			name: "only flags no command",
			args: []string{"--account=ACC123"},
			expected: parsedArgs{
				command: "",
				args:    nil,
				opFlags: []string{"--account=ACC123"},
			},
		},
		{
			name: "flag with special characters",
			args: []string{"--account=YOPUYSOQIRHYVGIV3IQ5CS627Y", "read", "op://Private/item/field"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://Private/item/field"},
				opFlags: []string{"--account=YOPUYSOQIRHYVGIV3IQ5CS627Y"},
			},
		},
		{
			name: "mixed flag formats",
			args: []string{"--account=ACC1", "read", "--account", "ACC2", "op://vault/item/field"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item/field"},
				opFlags: []string{"--account=ACC1", "--account=ACC2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseArgs(tt.args)

			if result.command != tt.expected.command {
				t.Errorf("command: got %q, want %q", result.command, tt.expected.command)
			}

			if !reflect.DeepEqual(result.args, tt.expected.args) {
				t.Errorf("args: got %v, want %v", result.args, tt.expected.args)
			}

			if !reflect.DeepEqual(result.opFlags, tt.expected.opFlags) {
				t.Errorf("opFlags: got %v, want %v", result.opFlags, tt.expected.opFlags)
			}
		})
	}
}

func TestParseArgsEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected parsedArgs
	}{
		{
			name: "double dash separator",
			args: []string{"run", "--env", "KEY=op://vault/item/field", "--", "echo", "test"},
			expected: parsedArgs{
				command: "run",
				args:    []string{"--env", "KEY=op://vault/item/field", "--", "echo", "test"},
				opFlags: nil,
			},
		},
		{
			name: "account flag after double dash still extracted",
			args: []string{"run", "--", "echo", "--account=ACC123"},
			expected: parsedArgs{
				command: "run",
				args:    []string{"--", "echo"},
				opFlags: []string{"--account=ACC123"},
			},
		},
		{
			name: "unknown flags ignored",
			args: []string{"--verbose", "read", "op://vault/item/field"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item/field"},
				opFlags: nil,
			},
		},
		{
			name: "account flag at end without value",
			args: []string{"read", "op://vault/item/field", "--account"},
			expected: parsedArgs{
				command: "read",
				args:    []string{"op://vault/item/field", "--account"},
				opFlags: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseArgs(tt.args)

			if result.command != tt.expected.command {
				t.Errorf("command: got %q, want %q", result.command, tt.expected.command)
			}

			if !reflect.DeepEqual(result.args, tt.expected.args) {
				t.Errorf("args: got %v, want %v", result.args, tt.expected.args)
			}

			if !reflect.DeepEqual(result.opFlags, tt.expected.opFlags) {
				t.Errorf("opFlags: got %v, want %v", result.opFlags, tt.expected.opFlags)
			}
		})
	}
}

func BenchmarkParseArgs(b *testing.B) {
	args := []string{"--account=ACC123", "read", "op://vault/item1/field", "op://vault/item2/field", "--account=ACC456"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = parseArgs(args)
	}
}
