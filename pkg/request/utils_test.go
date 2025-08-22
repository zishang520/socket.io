package request

import (
	"testing"
)

func TestSanitizeCookieName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal cookie name",
			input:    "sessionId",
			expected: "sessionId",
		},
		{
			name:     "Cookie name with newline",
			input:    "session\nId",
			expected: "session-Id",
		},
		{
			name:     "Cookie name with carriage return",
			input:    "session\rId",
			expected: "session-Id",
		},
		{
			name:     "Cookie name with both newline and carriage return",
			input:    "session\r\nId",
			expected: "session--Id",
		},
		{
			name:     "Empty cookie name",
			input:    "",
			expected: "",
		},
		{
			name:     "Multiple consecutive newlines",
			input:    "session\n\nId",
			expected: "session--Id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeCookieName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeCookieName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeCookieValue(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		quoted   bool
		expected string
	}{
		{
			name:     "Simple value",
			value:    "simple",
			quoted:   false,
			expected: "simple",
		},
		{
			name:     "Value with space",
			value:    "hello world",
			quoted:   false,
			expected: `"hello world"`,
		},
		{
			name:     "Value with comma",
			value:    "value1,value2",
			quoted:   false,
			expected: `"value1,value2"`,
		},
		{
			name:     "Empty value",
			value:    "",
			quoted:   false,
			expected: "",
		},
		{
			name:     "Forced quoted value",
			value:    "simple",
			quoted:   true,
			expected: `"simple"`,
		},
		{
			name:     "Value with invalid characters",
			value:    "hello\x00world",
			quoted:   false,
			expected: "helloworld",
		},
		{
			name:     "Value with quotes",
			value:    `hello"world`,
			quoted:   false,
			expected: "helloworld",
		},
		{
			name:     "Value with semicolon",
			value:    "hello;world",
			quoted:   false,
			expected: "helloworld",
		},
		{
			name:     "Value with backslash",
			value:    `hello\world`,
			quoted:   false,
			expected: "helloworld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeCookieValue(tt.value, tt.quoted)
			if result != tt.expected {
				t.Errorf("SanitizeCookieValue(%q, %v) = %q, want %q", tt.value, tt.quoted, result, tt.expected)
			}
		})
	}
}

func TestValidCookieValueByte(t *testing.T) {
	tests := []struct {
		name     string
		input    byte
		expected bool
	}{
		{
			name:     "Valid ASCII letter",
			input:    'a',
			expected: true,
		},
		{
			name:     "Valid ASCII number",
			input:    '5',
			expected: true,
		},
		{
			name:     "Valid special character",
			input:    '!',
			expected: true,
		},
		{
			name:     "Invalid control character",
			input:    0x1F,
			expected: false,
		},
		{
			name:     "Invalid double quote",
			input:    '"',
			expected: false,
		},
		{
			name:     "Invalid semicolon",
			input:    ';',
			expected: false,
		},
		{
			name:     "Invalid backslash",
			input:    '\\',
			expected: false,
		},
		{
			name:     "Invalid DEL character",
			input:    0x7F,
			expected: false,
		},
		{
			name:     "Space character",
			input:    ' ',
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validCookieValueByte(tt.input)
			if result != tt.expected {
				t.Errorf("validCookieValueByte(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeOrWarn(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		input     string
		expected  string
	}{
		{
			name:      "Valid input",
			fieldName: "test",
			input:     "hello",
			expected:  "hello",
		},
		{
			name:      "Input with invalid bytes",
			fieldName: "test",
			input:     "hello\x00world",
			expected:  "helloworld",
		},
		{
			name:      "Input with all valid bytes",
			fieldName: "test",
			input:     "!#$%&'()*+-./",
			expected:  "!#$%&'()*+-./",
		},
		{
			name:      "Empty input",
			fieldName: "test",
			input:     "",
			expected:  "",
		},
		{
			name:      "Input with multiple invalid bytes",
			fieldName: "test",
			input:     "hello\x00\x01\x02world",
			expected:  "helloworld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeOrWarn(tt.fieldName, validCookieValueByte, tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeOrWarn(%q, validCookieValueByte, %q) = %q, want %q",
					tt.fieldName, tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizationIntegration(t *testing.T) {
	tests := []struct {
		name          string
		cookieName    string
		cookieValue   string
		quoted        bool
		expectedName  string
		expectedValue string
	}{
		{
			name:          "Normal cookie",
			cookieName:    "session",
			cookieValue:   "abc123",
			quoted:        false,
			expectedName:  "session",
			expectedValue: "abc123",
		},
		{
			name:          "Cookie with invalid characters in both name and value",
			cookieName:    "session\nid\r",
			cookieValue:   "abc\x00123\x01",
			quoted:        false,
			expectedName:  "session-id-",
			expectedValue: "abc123",
		},
		{
			name:          "Cookie with spaces and quotes",
			cookieName:    "user\npreference",
			cookieValue:   "hello world",
			quoted:        true,
			expectedName:  "user-preference",
			expectedValue: `"hello world"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitizedName := SanitizeCookieName(tt.cookieName)
			if sanitizedName != tt.expectedName {
				t.Errorf("SanitizeCookieName(%q) = %q, want %q",
					tt.cookieName, sanitizedName, tt.expectedName)
			}

			sanitizedValue := SanitizeCookieValue(tt.cookieValue, tt.quoted)
			if sanitizedValue != tt.expectedValue {
				t.Errorf("SanitizeCookieValue(%q, %v) = %q, want %q",
					tt.cookieValue, tt.quoted, sanitizedValue, tt.expectedValue)
			}
		})
	}
}
