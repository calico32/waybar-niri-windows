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
		return "\033[1;33mwarning\033[0m"
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

func SetOutput(w io.Writer) {
	global.SetOutput(w)
}

func SetPrefix(prefix string) {
	global.SetPrefix(prefix)
}

func Tracef(format string, args ...any) {
	global.Tracef(format, args...)
}

func Debugf(format string, args ...any) {
	global.Debugf(format, args...)
}

func Infof(format string, args ...any) {
	global.Infof(format, args...)
}

func Warnf(format string, args ...any) {
	global.Warnf(format, args...)
}

func Errorf(format string, args ...any) {
	global.Errorf(format, args...)
}
