package safestring

import (
	"crypto/subtle"
	"unsafe"
)

// SafeString represents a string value stored in a byte slice that can be securely zeroed
type SafeString struct {
	data []byte
}

// New creates a SafeString from a regular string
func New(s string) *SafeString {
	data := make([]byte, len(s))
	copy(data, s)
	return &SafeString{data: data}
}

// FromBytes creates a SafeString from a byte slice (takes ownership)
func FromBytes(b []byte) *SafeString {
	data := make([]byte, len(b))
	copy(data, b)
	return &SafeString{data: data}
}

// String returns the string value (creates a copy)
func (s *SafeString) String() string {
	if s == nil || s.data == nil {
		return ""
	}
	return string(s.data)
}

// Bytes returns a copy of the underlying bytes
func (s *SafeString) Bytes() []byte {
	if s == nil || s.data == nil {
		return nil
	}
	result := make([]byte, len(s.data))
	copy(result, s.data)
	return result
}

// Len returns the length of the string
func (s *SafeString) Len() int {
	if s == nil || s.data == nil {
		return 0
	}
	return len(s.data)
}

// IsEmpty returns true if the string is empty
func (s *SafeString) IsEmpty() bool {
	return s.Len() == 0
}

// Equal compares two SafeStrings using constant-time comparison
func (s *SafeString) Equal(other *SafeString) bool {
	if s == nil || other == nil {
		return s == other
	}
	if s.data == nil || other.data == nil {
		return s.data == nil && other.data == nil
	}
	return subtle.ConstantTimeCompare(s.data, other.data) == 1
}

// EqualString compares SafeString with a regular string using constant-time comparison
func (s *SafeString) EqualString(str string) bool {
	if s == nil || s.data == nil {
		return str == ""
	}
	return subtle.ConstantTimeCompare(s.data, []byte(str)) == 1
}

// Clone creates a deep copy of the SafeString
func (s *SafeString) Clone() *SafeString {
	if s == nil || s.data == nil {
		return New("")
	}
	return FromBytes(s.data)
}

// Zero securely overwrites the underlying byte slice with zeros
func (s *SafeString) Zero() {
	if s == nil || s.data == nil {
		return
	}

	// Securely zero the memory
	for i := range s.data {
		s.data[i] = 0
	}

	// Additional security: use unsafe to ensure memory is overwritten
	if len(s.data) > 0 {
		ptr := unsafe.Pointer(&s.data[0])
		size := len(s.data)

		// Fill with zeros using unsafe operations for better security
		for i := 0; i < size; i++ {
			*(*byte)(unsafe.Pointer(uintptr(ptr) + uintptr(i))) = 0
		}
	}

	// Clear the slice reference
	s.data = nil
}

// Append safely appends bytes to the SafeString
func (s *SafeString) Append(b []byte) {
	if s == nil {
		return
	}
	if s.data == nil {
		s.data = make([]byte, 0, len(b))
	}
	s.data = append(s.data, b...)
}

// AppendString safely appends a string to the SafeString
func (s *SafeString) AppendString(str string) {
	s.Append([]byte(str))
}

// Truncate reduces the SafeString to the specified length, zeroing the removed portion
func (s *SafeString) Truncate(length int) {
	if s == nil || s.data == nil {
		return
	}

	if length >= len(s.data) {
		return // Nothing to truncate
	}

	if length < 0 {
		length = 0
	}

	// Zero the portion being removed
	for i := length; i < len(s.data); i++ {
		s.data[i] = 0
	}

	// Resize the slice
	s.data = s.data[:length]
}

// Pool manages a pool of SafeString instances for performance
type Pool struct {
	pool chan *SafeString
}

// NewPool creates a new SafeString pool with the specified capacity
func NewPool(capacity int) *Pool {
	return &Pool{
		pool: make(chan *SafeString, capacity),
	}
}

// Get retrieves a SafeString from the pool or creates a new one
func (p *Pool) Get() *SafeString {
	select {
	case s := <-p.pool:
		return s
	default:
		return &SafeString{}
	}
}

// Put returns a SafeString to the pool after zeroing it
func (p *Pool) Put(s *SafeString) {
	if s == nil {
		return
	}

	s.Zero()

	select {
	case p.pool <- s:
	default:
		// Pool is full, let it be garbage collected
	}
}
