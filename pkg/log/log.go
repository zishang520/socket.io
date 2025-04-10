package log

import (
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/gookit/color"
)

// Log flags that control the output format
const (
	Ldate         int = log.Ldate         // Print the date in the local time zone
	Ltime         int = log.Ltime         // Print the time in the local time zone
	Lmicroseconds int = log.Lmicroseconds // Print the time with microsecond precision
	Llongfile     int = log.Llongfile     // Print full file name and line number
	Lshortfile    int = log.Lshortfile    // Print final file name element and line number
	LUTC          int = log.LUTC          // Print date and time in UTC
	Lmsgprefix    int = log.Lmsgprefix    // Move the prefix from the beginning of the line to before the message
	LstdFlags     int = log.LstdFlags     // Standard flags: Ldate | Ltime
)

// Global configuration variables
var (
	DEBUG  bool      = false     // Global debug flag
	Output io.Writer = os.Stderr // Default output writer
	Prefix string    = ""        // Default prefix for all loggers
	Flags  int       = 0         // Default flags for all loggers
)

// Log represents a logger instance with enhanced functionality
type Log struct {
	*log.Logger

	prefix          atomic.Pointer[string] // Thread-safe prefix storage
	namespaceRegexp *regexp.Regexp         // Regexp for namespace-based debug filtering
}

// NewLog creates a new logger instance with the specified prefix
func NewLog(prefix string) *Log {
	l := &Log{
		Logger: log.New(Output, Prefix, Flags),
	}

	if prefix != "" {
		l.SetPrefix(prefix)
	}

	// Initialize namespace-based debug filtering if DEBUG environment variable is set
	if debug := os.Getenv("DEBUG"); debug != "" {
		l.namespaceRegexp = regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(strings.TrimSpace(debug)), `\*`, `.*`) + "$")
	}

	return l
}

// checkNamespace verifies if the current namespace matches the debug filter pattern
func (d *Log) checkNamespace(namespace string) bool {
	if d.namespaceRegexp != nil {
		return d.namespaceRegexp.MatchString(namespace)
	}
	return false
}

// Printlnf prints a formatted message with color support
func (d *Log) Printlnf(message string, args ...any) {
	d.Logger.Println(color.Sprintf(message, args...))
}

// Println is an alias for Printlnf
func (d *Log) Println(message string, args ...any) {
	d.Printlnf(message, args...)
}

// Defaultf prints a formatted message with default color
func (d *Log) Defaultf(message string, args ...any) {
	d.Logger.Println(color.Tag("default").Sprintf(message, args...))
}

// Default is an alias for Defaultf
func (d *Log) Default(message string, args ...any) {
	d.Defaultf(message, args...)
}

// Infof prints a formatted message with info color
func (d *Log) Infof(message string, args ...any) {
	d.Logger.Println(color.Info.Sprintf(message, args...))
}

// Info is an alias for Infof
func (d *Log) Info(message string, args ...any) {
	d.Infof(message, args...)
}

// Debugf prints a formatted message with debug color if debug mode is enabled
func (d *Log) Debugf(message string, args ...any) {
	if DEBUG && d.checkNamespace(d.Prefix()) {
		d.Logger.Println(color.Debug.Sprintf(message, args...))
	}
}

// Debug is an alias for Debugf that takes a simple message
func (d *Log) Debug(message string, args ...any) {
	d.Debugf(message, args...)
}

// Successf prints a formatted message with success color
func (d *Log) Successf(message string, args ...any) {
	d.Logger.Println(color.Success.Sprintf(message, args...))
}

// Success is an alias for Successf
func (d *Log) Success(message string, args ...any) {
	d.Successf(message, args...)
}

// Errorf prints a formatted message with error color
func (d *Log) Errorf(message string, args ...any) {
	d.Logger.Println(color.Danger.Sprintf(message, args...))
}

// Error is an alias for Errorf
func (d *Log) Error(message string, args ...any) {
	d.Errorf(message, args...)
}

// Warningf prints a formatted message with warning color
func (d *Log) Warningf(message string, args ...any) {
	d.Logger.Println(color.Warn.Sprintf(message, args...))
}

// Warning is an alias for Warningf
func (d *Log) Warning(message string, args ...any) {
	d.Warningf(message, args...)
}

// Secondaryf prints a formatted message with secondary color
func (d *Log) Secondaryf(message string, args ...any) {
	d.Logger.Println(color.Secondary.Sprintf(message, args...))
}

// Secondary is an alias for Secondaryf
func (d *Log) Secondary(message string, args ...any) {
	d.Secondaryf(message, args...)
}

// Questionf prints a formatted message with question color
func (d *Log) Questionf(message string, args ...any) {
	d.Logger.Println(color.Question.Sprintf(message, args...))
}

// Question is an alias for Questionf
func (d *Log) Question(message string, args ...any) {
	d.Questionf(message, args...)
}

// Fatalf prints a formatted message with error color and exits the program
func (d *Log) Fatalf(message string, args ...any) {
	d.Logger.Fatal(color.Error.Sprintf(message, args...))
}

// Fatal is an alias for Fatalf
func (d *Log) Fatal(message string, args ...any) {
	d.Fatalf(message, args...)
}

// Prefix returns the current logger prefix
func (d *Log) Prefix() string {
	if v := d.prefix.Load(); v != nil {
		return *v
	}
	return ""
}

// SetPrefix sets a new prefix for the logger
func (d *Log) SetPrefix(prefix string) {
	d.prefix.Store(&prefix)
	d.Logger.SetPrefix(prefix + " ")
}
