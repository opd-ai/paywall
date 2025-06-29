// Package wallet provides Bitcoin wallet functionality including address generation and encoding
package wallet

import (
	"bytes"
	"testing"
)

// TestBase58Encode_EmptyInput tests encoding of empty byte slice
func TestBase58Encode_EmptyInput(t *testing.T) {
	input := []byte{}
	result := Base58Encode(input)
	if result != "" {
		t.Errorf("Expected empty string for empty input, got %q", result)
	}
}

// TestBase58Encode_SingleZero tests encoding of single zero byte
func TestBase58Encode_SingleZero(t *testing.T) {
	input := []byte{0}
	result := Base58Encode(input)
	expected := "1"
	if result != expected {
		t.Errorf("Expected %q for single zero byte, got %q", expected, result)
	}
}

// TestBase58Encode_MultipleLeadingZeros tests encoding with multiple leading zeros
func TestBase58Encode_MultipleLeadingZeros(t *testing.T) {
	input := []byte{0, 0, 0, 60, 23, 110}
	result := Base58Encode(input)
	// Should preserve leading zeros as '1' characters
	if result[:3] != "111" {
		t.Errorf("Expected result to start with '111' for three leading zeros, got %q", result)
	}
}

// TestBase58Encode_KnownValues tests encoding with known test vectors
func TestBase58Encode_KnownValues(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "hello world",
			input:    []byte("hello world"),
			expected: "StV1DL6CwTryKyV",
		},
		{
			name:     "simple bytes",
			input:    []byte{0x00, 0x61, 0xbc, 0x66, 0x49, 0x69, 0x59, 0x62, 0x01, 0x5b},
			expected: "12F9zDcnkbB3N2",
		},
		{
			name:     "bitcoin address payload",
			input:    []byte{0x00, 0x01, 0x02, 0x03},
			expected: "1Ldp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Base58Encode(tt.input)
			if result != tt.expected {
				t.Errorf("Base58Encode() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

// TestBase58Decode_EmptyInput tests decoding of empty string
func TestBase58Decode_EmptyInput(t *testing.T) {
	input := ""
	result, err := Base58Decode(input)
	if err != nil {
		t.Errorf("Expected no error for empty input, got %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty byte slice for empty input, got %v", result)
	}
}

// TestBase58Decode_SingleOne tests decoding of single '1' character
func TestBase58Decode_SingleOne(t *testing.T) {
	input := "1"
	result, err := Base58Decode(input)
	if err != nil {
		t.Errorf("Expected no error for single '1', got %v", err)
	}
	expected := []byte{0}
	if !bytes.Equal(result, expected) {
		t.Errorf("Expected %v for single '1', got %v", expected, result)
	}
}

// TestBase58Decode_InvalidCharacters tests decoding with invalid characters
func TestBase58Decode_InvalidCharacters(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"zero character", "0"},
		{"capital O", "O"},
		{"capital I", "I"},
		{"lowercase l", "l"},
		{"special character", "!"},
		{"space character", " "},
		{"mixed invalid", "1O2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Base58Decode(tt.input)
			if err == nil {
				t.Errorf("Expected error for invalid input %q, got nil", tt.input)
			}
			if err.Error() != "invalid base58 character" {
				t.Errorf("Expected 'invalid base58 character' error, got %q", err.Error())
			}
		})
	}
}

// TestBase58Decode_KnownValues tests decoding with known test vectors
func TestBase58Decode_KnownValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []byte
	}{
		{
			name:     "hello world",
			input:    "StV1DL6CwTryKyV",
			expected: []byte("hello world"),
		},
		{
			name:     "bitcoin address payload",
			input:    "12F9zDcnkbB3N2",
			expected: []byte{0x00, 0x61, 0xbc, 0x66, 0x49, 0x69, 0x59, 0x62, 0x01, 0x5b},
		},
		{
			name:     "simple encoding",
			input:    "1Ldp",
			expected: []byte{0x00, 0x01, 0x02, 0x03},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Base58Decode(tt.input)
			if err != nil {
				t.Errorf("Base58Decode() error = %v", err)
				return
			}
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("Base58Decode() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestBase58Decode_MultipleLeadingOnes tests decoding with multiple leading '1' characters
func TestBase58Decode_MultipleLeadingOnes(t *testing.T) {
	input := "111StV1DL6CwTryKyV"
	result, err := Base58Decode(input)
	if err != nil {
		t.Errorf("Expected no error for input with leading ones, got %v", err)
	}
	
	// Should preserve leading zeros
	if len(result) < 3 || result[0] != 0 || result[1] != 0 || result[2] != 0 {
		t.Errorf("Expected result to start with three zero bytes, got %v", result)
	}
	
	// Rest should decode to "hello world"
	expected := append([]byte{0, 0, 0}, []byte("hello world")...)
	if !bytes.Equal(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

// TestBase58EncodeDecodeRoundTrip tests that encoding then decoding returns original data
func TestBase58EncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty", []byte{}},
		{"single zero", []byte{0}},
		{"multiple zeros", []byte{0, 0, 0}},
		{"mixed with leading zeros", []byte{0, 0, 1, 2, 3}},
		{"no leading zeros", []byte{1, 2, 3, 4, 5}},
		{"hello world", []byte("hello world")},
		{"bitcoin-like data", []byte{0x00, 0x61, 0xbc, 0x66, 0x49, 0x69, 0x59, 0x62, 0x01, 0x5b}},
		{"max byte values", []byte{0xff, 0xff, 0xff}},
		{"mixed bytes", []byte{0x00, 0x01, 0x80, 0xff}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode then decode
			encoded := Base58Encode(tt.input)
			decoded, err := Base58Decode(encoded)
			
			if err != nil {
				t.Errorf("Round trip failed with error: %v", err)
				return
			}
			
			if !bytes.Equal(decoded, tt.input) {
				t.Errorf("Round trip failed: input=%v, encoded=%q, decoded=%v", tt.input, encoded, decoded)
			}
		})
	}
}

// TestBase58AlphabetCoverage tests that all alphabet characters are handled correctly
func TestBase58AlphabetCoverage(t *testing.T) {
	alphabet := "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	
	// Test each character individually
	for _, char := range alphabet {
		t.Run(string(char), func(t *testing.T) {
			input := string(char)
			decoded, err := Base58Decode(input)
			if err != nil {
				t.Errorf("Valid alphabet character %q failed to decode: %v", char, err)
				return
			}
			
			// Re-encode and verify we get the same character
			encoded := Base58Encode(decoded)
			if encoded != input {
				t.Errorf("Character %q round trip failed: got %q", char, encoded)
			}
		})
	}
}

// TestBase58LargeNumbers tests encoding/decoding of large numbers
func TestBase58LargeNumbers(t *testing.T) {
	// Large byte array representing a big number
	largeBytes := make([]byte, 32)
	for i := range largeBytes {
		largeBytes[i] = byte(i + 1) // Avoid zero to test large number handling
	}
	
	encoded := Base58Encode(largeBytes)
	decoded, err := Base58Decode(encoded)
	
	if err != nil {
		t.Errorf("Large number decode failed: %v", err)
		return
	}
	
	if !bytes.Equal(decoded, largeBytes) {
		t.Errorf("Large number round trip failed")
	}
}
