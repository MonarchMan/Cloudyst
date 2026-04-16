package logging

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/fatih/color"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/gofrs/uuid"
)

const (
	green   = "\033[97;42m"
	white   = "\033[90;47m"
	yellow  = "\033[90;43m"
	red     = "\033[97;41m"
	blue    = "\033[97;44m"
	magenta = "\033[97;45m"
	cyan    = "\033[97;46m"
	reset   = "\033[0m"
)

// Logger interface for logging messages.
type Logger interface {
	Panic(format string, v ...any)
	Error(format string, v ...any)
	Warn(format string, v ...any)
	Info(format string, v ...any)
	Debug(format string, v ...any)
	// Copy a new logger with a prefix.
	CopyWithPrefix(prefix string) Logger

	// SupportColor returns if current logger support outputting colors.
	SupportColor() bool
}

// LoggerCtx defines keys for logger with correlation ID
type LoggerCtx struct{}

// CorrelationIDCtx defines keys for correlation ID
type CorrelationIDCtx struct{}
type LogLevel string

const (
	// LevelError 错误
	LevelError LogLevel = "error"
	// LevelWarning 警告
	LevelWarning LogLevel = "warning"
	// LevelInformational 提示
	LevelInformational LogLevel = "info"
	// LevelDebug 除错
	LevelDebug LogLevel = "debug"
)

// NewConsoleLogger initializes a new logging that prints logs to Stdout.
func NewConsoleLogger(level LogLevel) Logger {
	logFunc := func(level string) loggingFunc {
		return func(logger *consoleLogger, s string, a ...any) {
			msg := fmt.Sprintf(s, a...)
			logger.println(level, msg)
		}
	}

	logger := &consoleLogger{
		warning: logFunc("Warn"),
		panic: func(logger *consoleLogger, s string, a ...any) {
			msg := fmt.Sprintf(s, a...)
			logger.println("Panic", msg)
			panic(msg)
		},
		error: logFunc("Error"),
		info:  logFunc("Info"),
		debug: logFunc("Debug"),
	}

	switch level {
	case LevelError:
		logger.warning = noopLoggingFunc
		logger.info = noopLoggingFunc
		logger.debug = noopLoggingFunc
	case LevelWarning:
		logger.info = noopLoggingFunc
		logger.debug = noopLoggingFunc
	case LevelInformational:
		logger.debug = noopLoggingFunc
	case LevelDebug:

	}

	return logger
}

// FromContext retrieves a logger from context.
func FromContext(ctx context.Context) log.Logger {
	v, ok := ctx.Value(LoggerCtx{}).(log.Logger)
	if !ok {
		v = log.DefaultLogger
	}
	return v
}

// CorrelationID retrieves a correlation ID from context.
func CorrelationID(ctx context.Context) uuid.UUID {
	v, ok := ctx.Value(CorrelationIDCtx{}).(uuid.UUID)
	if !ok {
		v = uuid.Nil
	}
	return v
}

type consoleLogger struct {
	warning loggingFunc
	panic   loggingFunc
	error   loggingFunc
	info    loggingFunc
	debug   loggingFunc
	prefix  string
}

func (ll *consoleLogger) Panic(format string, v ...any) {
	ll.panic(ll, format, v...)
}

func (ll *consoleLogger) Error(format string, v ...any) {
	ll.error(ll, format, v...)
}

func (ll *consoleLogger) Warn(format string, v ...any) {
	ll.warning(ll, format, v...)
}

func (ll *consoleLogger) Info(format string, v ...any) {
	ll.info(ll, format, v...)
}

func (ll *consoleLogger) Debug(format string, v ...any) {
	ll.debug(ll, format, v...)
}

// println 打印
func (ll *consoleLogger) println(level string, msg string) {
	c := color.New()
	_, filename, line, _ := runtime.Caller(3)

	_, _ = c.Printf(
		"%s\t %s [%s:%d]%s %s\n",
		colors[level]("["+level+"]"),
		time.Now().Format("2006-01-02 15:04:05"),
		filename,
		line,
		ll.prefix,
		msg,
	)
}

func (ll *consoleLogger) CopyWithPrefix(prefix string) Logger {
	return &consoleLogger{
		warning: ll.warning,
		panic:   ll.panic,
		error:   ll.error,
		info:    ll.info,
		debug:   ll.debug,
		prefix:  ll.prefix + " " + prefix,
	}
}

func (ll *consoleLogger) SupportColor() bool {
	return !color.NoColor
}

type loggingFunc func(*consoleLogger, string, ...any)

func noopLoggingFunc(*consoleLogger, string, ...any) {}

var colors = map[string]func(a ...interface{}) string{
	"Warn":  color.New(color.FgYellow).Add(color.Bold).SprintFunc(),
	"Panic": color.New(color.BgRed).Add(color.Bold).SprintFunc(),
	"Error": color.New(color.FgRed).Add(color.Bold).SprintFunc(),
	"Info":  color.New(color.FgCyan).Add(color.Bold).SprintFunc(),
	"Debug": color.New(color.FgWhite).Add(color.Bold).SprintFunc(),
}

// Request helper fund to log request.
func Request(l log.Logger, incoming bool, code int, method, clientIP, path, err string, start time.Time) {
	var statusColor, methodColor, resetColor string
	if SupportColor(l) {
		statusColor = getStatusCodeColor(code)
		methodColor = getMethodColor(method)
		resetColor = "\033[0m"
	}

	category := "Incoming"
	if !incoming {
		category = "Outgoing"
	}
	h := log.NewHelper(l, log.WithMessageKey("request"))

	h.Info(
		"[%s] %s %3d %s| %13v | %15s |%s %-7s %s %#v",
		category,
		statusColor, code, resetColor,
		time.Now().Sub(start),
		clientIP,
		methodColor, method, resetColor,
		path,
	)
	if err != "" {
		h.Error("%s", err)
	}
}

func SupportColor(l log.Logger) bool {
	return !color.NoColor
}

// StatusCodeColor is the ANSI color for appropriately logging http status code to a terminal.
func getStatusCodeColor(code int) string {
	switch {
	case code >= http.StatusContinue && code < http.StatusOK:
		return white
	case code >= http.StatusOK && code < http.StatusMultipleChoices:
		return green
	case code >= http.StatusMultipleChoices && code < http.StatusBadRequest:
		return white
	case code >= http.StatusBadRequest && code < http.StatusInternalServerError:
		return yellow
	default:
		return red
	}
}

// MethodColor is the ANSI color for appropriately logging http method to a terminal.
func getMethodColor(method string) string {
	switch method {
	case http.MethodGet:
		return blue
	case http.MethodPost:
		return cyan
	case http.MethodPut:
		return yellow
	case http.MethodDelete:
		return red
	case http.MethodPatch:
		return green
	case http.MethodHead:
		return magenta
	case http.MethodOptions:
		return white
	default:
		return reset
	}
}
