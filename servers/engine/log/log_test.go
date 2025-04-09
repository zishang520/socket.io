package log

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestNewLog(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		envDebug string
		want     string
	}{
		{
			name:     "without prefix",
			prefix:   "",
			envDebug: "",
			want:     "",
		},
		{
			name:     "with prefix",
			prefix:   "test",
			envDebug: "",
			want:     "test",
		},
		{
			name:     "with debug env",
			prefix:   "test",
			envDebug: "test*",
			want:     "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envDebug != "" {
				os.Setenv("DEBUG", tt.envDebug)
				defer os.Unsetenv("DEBUG")
			}

			logger := NewLog(tt.prefix)
			if got := logger.Prefix(); got != tt.want {
				t.Errorf("NewLog() prefix = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLogMethods(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf
	defer func() { Output = os.Stderr }()

	logger := NewLog("test")

	tests := []struct {
		name     string
		fn       func()
		contains string
	}{
		{
			name: "Println",
			fn: func() {
				logger.Println("test message")
			},
			contains: "test message",
		},
		{
			name: "Info",
			fn: func() {
				logger.Info("info message")
			},
			contains: "info message",
		},
		{
			name: "Error",
			fn: func() {
				logger.Error("error message")
			},
			contains: "error message",
		},
		{
			name: "Warning",
			fn: func() {
				logger.Warning("warning message")
			},
			contains: "warning message",
		},
		{
			name: "Success",
			fn: func() {
				logger.Success("success message")
			},
			contains: "success message",
		},
		{
			name: "Debug",
			fn: func() {
				logger.Debug("debug message")
			},
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.fn()
			got := buf.String()
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("%s() output = %v, want to contain %v", tt.name, got, tt.contains)
			}
		})
	}
}

func TestDebugWithNamespace(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf
	defer func() { Output = os.Stderr }()

	DEBUG = true // Enable debug mode for testing
	defer func() { DEBUG = false }()

	tests := []struct {
		name     string
		prefix   string
		envDebug string
		message  string
		contains string
	}{
		{
			name:     "matching namespace",
			prefix:   "test",
			envDebug: "test*",
			message:  "debug message",
			contains: "debug message",
		},
		{
			name:     "non-matching namespace",
			prefix:   "other",
			envDebug: "test*",
			message:  "debug message",
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DEBUG", tt.envDebug)
			defer os.Unsetenv("DEBUG")

			logger := NewLog(tt.prefix)
			buf.Reset()
			logger.Debug("%s", tt.message)

			got := buf.String()
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("Debug() output = %v, want to contain %v", got, tt.contains)
			}
		})
	}
}

func TestSetPrefix(t *testing.T) {
	logger := NewLog("initial")
	if got := logger.Prefix(); got != "initial" {
		t.Errorf("Initial prefix = %v, want %v", got, "initial")
	}

	logger.SetPrefix("new")
	if got := logger.Prefix(); got != "new" {
		t.Errorf("After SetPrefix() prefix = %v, want %v", got, "new")
	}
}

func TestFormattedMethods(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf
	defer func() { Output = os.Stderr }()

	logger := NewLog("test")

	tests := []struct {
		name     string
		fn       func()
		contains string
	}{
		{
			name: "Printlnf",
			fn: func() {
				logger.Printlnf("test %s", "message")
			},
			contains: "test message",
		},
		{
			name: "Infof",
			fn: func() {
				logger.Infof("info %s", "message")
			},
			contains: "info message",
		},
		{
			name: "Errorf",
			fn: func() {
				logger.Errorf("error %s", "message")
			},
			contains: "error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.fn()
			got := buf.String()
			if !strings.Contains(got, tt.contains) {
				t.Errorf("%s() output = %v, want to contain %v", tt.name, got, tt.contains)
			}
		})
	}
}

func TestLog(t *testing.T) {
	DEBUG = true
	os.Setenv("DEBUG", "")
	_log := NewLog("namespace")
	buf := new(bytes.Buffer)

	t.Run("prefix", func(t *testing.T) {
		if _log.Prefix() != "namespace" && _log.Logger.Prefix() == "namespace " {
			t.Fatalf(`*Log.Prefix() = %q, want match for %#q`, _log.Prefix(), "namespace")
		}
	})

	_log.SetFlags(0)
	_log.SetOutput(buf)

	_log.Debug("Test")

	if buf.Len() > 0 {
		t.Fatal(`*Log.Debug("Test") There should be no output here, but got the output.`)
	}

	buf.Reset()

	_log.Printf("hello %d world", 23)
	line := buf.String()
	line = line[0 : len(line)-1]
	pattern := "^" + _log.Logger.Prefix() + "hello 23 world$"
	matched, err := regexp.MatchString(pattern, line)
	if err != nil {
		t.Fatal("pattern did not compile:", err)
	}
	if !matched {
		t.Errorf("log output should match %q is %q", pattern, line)
	}
	_log.SetOutput(os.Stderr)
}
