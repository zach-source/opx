package protocol

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestReadRequest(t *testing.T) {
	tests := []struct {
		name     string
		req      ReadRequest
		expected string
	}{
		{
			name:     "simple request",
			req:      ReadRequest{Ref: "op://vault/item/field"},
			expected: `{"ref":"op://vault/item/field"}`,
		},
		{
			name:     "empty ref",
			req:      ReadRequest{Ref: ""},
			expected: `{"ref":""}`,
		},
		{
			name:     "complex ref",
			req:      ReadRequest{Ref: "op://vault/My Item/password"},
			expected: `{"ref":"op://vault/My Item/password"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Errorf("Failed to marshal: %v", err)
			}
			if string(data) != tt.expected {
				t.Errorf("Expected JSON %q, got %q", tt.expected, string(data))
			}

			// Test unmarshaling
			var unmarshaled ReadRequest
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Errorf("Failed to unmarshal: %v", err)
			}
			if !reflect.DeepEqual(unmarshaled, tt.req) {
				t.Errorf("Unmarshaled request %+v differs from original %+v", unmarshaled, tt.req)
			}
		})
	}
}

func TestReadsRequest(t *testing.T) {
	tests := []struct {
		name     string
		req      ReadsRequest
		expected string
	}{
		{
			name:     "multiple refs",
			req:      ReadsRequest{Refs: []string{"op://vault/item1/field1", "op://vault/item2/field2"}},
			expected: `{"refs":["op://vault/item1/field1","op://vault/item2/field2"]}`,
		},
		{
			name:     "single ref",
			req:      ReadsRequest{Refs: []string{"op://vault/item/field"}},
			expected: `{"refs":["op://vault/item/field"]}`,
		},
		{
			name:     "empty refs",
			req:      ReadsRequest{Refs: []string{}},
			expected: `{"refs":[]}`,
		},
		{
			name:     "nil refs",
			req:      ReadsRequest{Refs: nil},
			expected: `{"refs":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Errorf("Failed to marshal: %v", err)
			}
			if string(data) != tt.expected {
				t.Errorf("Expected JSON %q, got %q", tt.expected, string(data))
			}

			// Test unmarshaling
			var unmarshaled ReadsRequest
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Errorf("Failed to unmarshal: %v", err)
			}
			if !reflect.DeepEqual(unmarshaled, tt.req) {
				t.Errorf("Unmarshaled request %+v differs from original %+v", unmarshaled, tt.req)
			}
		})
	}
}

func TestReadResponse(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name     string
		resp     ReadResponse
		expected string
	}{
		{
			name: "complete response",
			resp: ReadResponse{
				Ref:        "op://vault/item/field",
				Value:      "secret-value",
				FromCache:  true,
				ExpiresIn:  300,
				ResolvedAt: now,
			},
			expected: fmt.Sprintf(`{"ref":"op://vault/item/field","value":"secret-value","from_cache":true,"expires_in_seconds":300,"resolved_at_unix":%d}`, now),
		},
		{
			name: "fresh response",
			resp: ReadResponse{
				Ref:        "op://vault/item/password",
				Value:      "password123",
				FromCache:  false,
				ExpiresIn:  600,
				ResolvedAt: now,
			},
			expected: fmt.Sprintf(`{"ref":"op://vault/item/password","value":"password123","from_cache":false,"expires_in_seconds":600,"resolved_at_unix":%d}`, now),
		},
		{
			name: "zero values",
			resp: ReadResponse{
				Ref:        "",
				Value:      "",
				FromCache:  false,
				ExpiresIn:  0,
				ResolvedAt: 0,
			},
			expected: `{"ref":"","value":"","from_cache":false,"expires_in_seconds":0,"resolved_at_unix":0}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.resp)
			if err != nil {
				t.Errorf("Failed to marshal: %v", err)
			}
			if string(data) != tt.expected {
				t.Errorf("Expected JSON %q, got %q", tt.expected, string(data))
			}

			// Test unmarshaling
			var unmarshaled ReadResponse
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Errorf("Failed to unmarshal: %v", err)
			}
			if !reflect.DeepEqual(unmarshaled, tt.resp) {
				t.Errorf("Unmarshaled response %+v differs from original %+v", unmarshaled, tt.resp)
			}
		})
	}
}

func TestReadsResponse(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name     string
		resp     ReadsResponse
		expected string
	}{
		{
			name: "multiple results",
			resp: ReadsResponse{
				Results: map[string]ReadResponse{
					"op://vault/item1/field1": {
						Ref:        "op://vault/item1/field1",
						Value:      "value1",
						FromCache:  true,
						ExpiresIn:  300,
						ResolvedAt: now,
					},
					"op://vault/item2/field2": {
						Ref:        "op://vault/item2/field2",
						Value:      "value2",
						FromCache:  false,
						ExpiresIn:  600,
						ResolvedAt: now,
					},
				},
			},
			// Note: map iteration order is not guaranteed in Go, so we test unmarshaling instead
			expected: "",
		},
		{
			name:     "empty results",
			resp:     ReadsResponse{Results: make(map[string]ReadResponse)},
			expected: `{"results":{}}`,
		},
		{
			name:     "nil results",
			resp:     ReadsResponse{Results: nil},
			expected: `{"results":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling and unmarshaling round-trip
			data, err := json.Marshal(tt.resp)
			if err != nil {
				t.Errorf("Failed to marshal: %v", err)
			}

			// For specific expected strings, test them
			if tt.expected != "" {
				if string(data) != tt.expected {
					t.Errorf("Expected JSON %q, got %q", tt.expected, string(data))
				}
			}

			// Test unmarshaling
			var unmarshaled ReadsResponse
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Errorf("Failed to unmarshal: %v", err)
			}
			if !reflect.DeepEqual(unmarshaled, tt.resp) {
				t.Errorf("Unmarshaled response %+v differs from original %+v", unmarshaled, tt.resp)
			}
		})
	}
}

func TestResolveRequest(t *testing.T) {
	tests := []struct {
		name     string
		req      ResolveRequest
		expected string
	}{
		{
			name: "multiple env vars",
			req: ResolveRequest{
				Env: map[string]string{
					"DB_PASSWORD": "op://vault/db/password",
					"API_KEY":     "op://vault/api/key",
				},
			},
			// Map order is not guaranteed, so we'll test round-trip
			expected: "",
		},
		{
			name: "single env var",
			req: ResolveRequest{
				Env: map[string]string{
					"PASSWORD": "op://vault/item/password",
				},
			},
			expected: `{"env":{"PASSWORD":"op://vault/item/password"}}`,
		},
		{
			name:     "empty env",
			req:      ResolveRequest{Env: make(map[string]string)},
			expected: `{"env":{}}`,
		},
		{
			name:     "nil env",
			req:      ResolveRequest{Env: nil},
			expected: `{"env":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Errorf("Failed to marshal: %v", err)
			}

			// For specific expected strings, test them
			if tt.expected != "" {
				if string(data) != tt.expected {
					t.Errorf("Expected JSON %q, got %q", tt.expected, string(data))
				}
			}

			// Test unmarshaling
			var unmarshaled ResolveRequest
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Errorf("Failed to unmarshal: %v", err)
			}
			if !reflect.DeepEqual(unmarshaled, tt.req) {
				t.Errorf("Unmarshaled request %+v differs from original %+v", unmarshaled, tt.req)
			}
		})
	}
}

func TestResolveResponse(t *testing.T) {
	tests := []struct {
		name     string
		resp     ResolveResponse
		expected string
	}{
		{
			name: "multiple env vars",
			resp: ResolveResponse{
				Env: map[string]string{
					"DB_PASSWORD": "secret-password",
					"API_KEY":     "secret-key",
				},
			},
			// Map order is not guaranteed
			expected: "",
		},
		{
			name: "single env var",
			resp: ResolveResponse{
				Env: map[string]string{
					"PASSWORD": "secret-value",
				},
			},
			expected: `{"env":{"PASSWORD":"secret-value"}}`,
		},
		{
			name:     "empty env",
			resp:     ResolveResponse{Env: make(map[string]string)},
			expected: `{"env":{}}`,
		},
		{
			name:     "nil env",
			resp:     ResolveResponse{Env: nil},
			expected: `{"env":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.resp)
			if err != nil {
				t.Errorf("Failed to marshal: %v", err)
			}

			// For specific expected strings, test them
			if tt.expected != "" {
				if string(data) != tt.expected {
					t.Errorf("Expected JSON %q, got %q", tt.expected, string(data))
				}
			}

			// Test unmarshaling
			var unmarshaled ResolveResponse
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Errorf("Failed to unmarshal: %v", err)
			}
			if !reflect.DeepEqual(unmarshaled, tt.resp) {
				t.Errorf("Unmarshaled response %+v differs from original %+v", unmarshaled, tt.resp)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   Status
		expected string
	}{
		{
			name: "complete status",
			status: Status{
				Backend:    "opcli",
				CacheSize:  10,
				Hits:       100,
				Misses:     50,
				InFlight:   2,
				TTLSeconds: 300,
				SocketPath: "/tmp/op-authd.sock",
			},
			expected: `{"backend":"opcli","cache_size":10,"hits":100,"misses":50,"in_flight":2,"ttl_seconds":300,"socket_path":"/tmp/op-authd.sock"}`,
		},
		{
			name: "fake backend",
			status: Status{
				Backend:    "fake",
				CacheSize:  0,
				Hits:       0,
				Misses:     0,
				InFlight:   0,
				TTLSeconds: 600,
				SocketPath: "/var/run/op-authd.sock",
			},
			expected: `{"backend":"fake","cache_size":0,"hits":0,"misses":0,"in_flight":0,"ttl_seconds":600,"socket_path":"/var/run/op-authd.sock"}`,
		},
		{
			name: "zero values",
			status: Status{
				Backend:    "",
				CacheSize:  0,
				Hits:       0,
				Misses:     0,
				InFlight:   0,
				TTLSeconds: 0,
				SocketPath: "",
			},
			expected: `{"backend":"","cache_size":0,"hits":0,"misses":0,"in_flight":0,"ttl_seconds":0,"socket_path":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.status)
			if err != nil {
				t.Errorf("Failed to marshal: %v", err)
			}
			if string(data) != tt.expected {
				t.Errorf("Expected JSON %q, got %q", tt.expected, string(data))
			}

			// Test unmarshaling
			var unmarshaled Status
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Errorf("Failed to unmarshal: %v", err)
			}
			if !reflect.DeepEqual(unmarshaled, tt.status) {
				t.Errorf("Unmarshaled status %+v differs from original %+v", unmarshaled, tt.status)
			}
		})
	}
}

// TestJSONFieldTags ensures all struct fields have correct JSON tags
func TestJSONFieldTags(t *testing.T) {
	// Test that all structs can be marshaled and unmarshaled without errors
	testStructs := []interface{}{
		ReadRequest{Ref: "test"},
		ReadsRequest{Refs: []string{"test1", "test2"}},
		ReadResponse{
			Ref: "test", Value: "value", FromCache: true,
			ExpiresIn: 100, ResolvedAt: 1234567890,
		},
		ReadsResponse{Results: map[string]ReadResponse{
			"key": {Ref: "key", Value: "value"},
		}},
		ResolveRequest{Env: map[string]string{"KEY": "ref"}},
		ResolveResponse{Env: map[string]string{"KEY": "value"}},
		Status{
			Backend: "test", CacheSize: 1, Hits: 2, Misses: 3,
			InFlight: 4, TTLSeconds: 5, SocketPath: "/test",
		},
	}

	for i, s := range testStructs {
		t.Run(reflect.TypeOf(s).Name(), func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(s)
			if err != nil {
				t.Errorf("Failed to marshal struct %d: %v", i, err)
			}

			// Unmarshal back into same type
			newVal := reflect.New(reflect.TypeOf(s)).Interface()
			if err := json.Unmarshal(data, newVal); err != nil {
				t.Errorf("Failed to unmarshal struct %d: %v", i, err)
			}

			// Compare (dereference the pointer)
			original := s
			unmarshaled := reflect.ValueOf(newVal).Elem().Interface()
			if !reflect.DeepEqual(original, unmarshaled) {
				t.Errorf("Struct %d: round-trip failed. Original: %+v, Unmarshaled: %+v",
					i, original, unmarshaled)
			}
		})
	}
}

// TestInvalidJSON tests handling of malformed JSON
func TestInvalidJSON(t *testing.T) {
	testCases := []struct {
		name    string
		jsonStr string
		target  interface{}
	}{
		{"ReadRequest invalid", `{"ref":123}`, &ReadRequest{}},
		{"ReadRequest malformed", `{"ref":}`, &ReadRequest{}},
		{"ReadsRequest invalid", `{"refs":"not-an-array"}`, &ReadsRequest{}},
		{"ReadResponse invalid", `{"expires_in_seconds":"not-a-number"}`, &ReadResponse{}},
		{"Status invalid", `{"hits":"not-a-number"}`, &Status{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := json.Unmarshal([]byte(tc.jsonStr), tc.target)
			if err == nil {
				t.Errorf("Expected error when unmarshaling invalid JSON: %s", tc.jsonStr)
			}
		})
	}
}

// Benchmark tests for JSON serialization performance
func BenchmarkReadRequestMarshal(b *testing.B) {
	req := ReadRequest{Ref: "op://vault/item/field"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(req)
	}
}

func BenchmarkReadResponseMarshal(b *testing.B) {
	resp := ReadResponse{
		Ref: "op://vault/item/field", Value: "secret-value",
		FromCache: true, ExpiresIn: 300, ResolvedAt: 1234567890,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(resp)
	}
}

func BenchmarkStatusMarshal(b *testing.B) {
	status := Status{
		Backend: "opcli", CacheSize: 10, Hits: 100, Misses: 50,
		InFlight: 2, TTLSeconds: 300, SocketPath: "/tmp/sock",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(status)
	}
}
