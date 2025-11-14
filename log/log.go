package log

import (
	"fmt"
	"io"
	"os"
	"time"
)

type Logger struct {
	output io.Writer
	prefix string
	level  Level
}

type Level int

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelTrace:
		return "\033[37mtrace\033[0m"
	case LevelDebug:
		return "\033[36mdebug\033[0m"
	case LevelInfo:
		return "\033[32minfo\033[0m"
	case LevelWarn:
		return "\033[1;33mwarn\033[0m"
	case LevelError:
		return "\033[1;31merror\033[0m"
	default:
		return "unknown"
	}
}

func (l *Logger) printf(level Level, format string, args ...any) {
	if l.level > level || l.output == nil {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Appendf(nil, format, args...)
	if len(msg) > 0 && msg[len(msg)-1] != '\n' {
		msg = append(msg, '\n')
	}
	fmt.Fprintf(l.output, "[%s] [%s] [%s] %s", timestamp, level, l.prefix, msg)
}

func (l *Logger) SetOutput(w io.Writer) {
	l.output = w
}

func (l *Logger) SetPrefix(prefix string) {
	l.prefix = prefix
}

func (l *Logger) Tracef(format string, args ...any) {
	l.printf(LevelTrace, format, args...)
}

func (l *Logger) Debugf(format string, args ...any) {
	l.printf(LevelDebug, format, args...)
}

func (l *Logger) Infof(format string, args ...any) {
	l.printf(LevelInfo, format, args...)
}

func (l *Logger) Warnf(format string, args ...any) {
	l.printf(LevelWarn, format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.printf(LevelError, format, args...)
}

var global = Logger{os.Stderr, "niri-windows", LevelInfo}

// SetOutput sets the output writer for the package-level Logger.
// If w is nil, logging is disabled.
func SetOutput(w io.Writer) {
	global.SetOutput(w)
}

// SetPrefix sets the prefix that is prepended to each log message emitted by the package logger.
func SetPrefix(prefix string) {
	global.SetPrefix(prefix)
}

// Tracef logs a formatted message at trace level using the package-level logger.
// The message is formatted with the provided format and args and will be written only if the logger's level and output permit it.
func Tracef(format string, args ...any) {
	global.Tracef(format, args...)
}

// Debugf formats a message using fmt.Sprintf semantics and logs it at the debug level using the package-level logger.
// The message is written only when the package logger's level permits debug output and when an output writer is configured.
func Debugf(format string, args ...any) {
	global.Debugf(format, args...)
}

// Infof formats according to format and arguments and logs the result at the Info level using the package-level logger.
func Infof(format string, args ...any) {
	global.Infof(format, args...)
}

// Warnf formats a warning-level message according to the format and args and writes it using the package-level logger.
// The formatting follows the conventions of fmt.Printf.
func Warnf(format string, args ...any) {
	global.Warnf(format, args...)
}

// Errorf formats according to format and args and logs the result at Error level using the package-level logger.
func Errorf(format string, args ...any) {
	global.Errorf(format, args...)
}