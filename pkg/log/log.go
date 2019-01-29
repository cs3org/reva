package log

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/rs/zerolog"
)

func init() {
	zerolog.CallerSkipFrameCount = 3
}

var pkgs = []string{}
var enabledLoggers = map[string]*zerolog.Logger{}

// Out is the log output writer
var Out io.Writer = os.Stderr

// Mode dev prints in console format and prod in json output
var Mode = "dev"

// Logger is the main logging element
type Logger struct {
	pkg string
	pid int
}

// ListRegisteredPackages returns the name of the packages a log has been registered.s
func ListRegisteredPackages() []string {
	return pkgs
}

// ListEnabledPackages returns a list with the name of log-enabled packages.
func ListEnabledPackages() []string {
	pkgs := []string{}
	for k := range enabledLoggers {
		pkgs = append(pkgs, k)
	}
	return pkgs
}

// EnableAll enables all registered loggers
func EnableAll() error {
	for _, v := range pkgs {
		if err := Enable(v); err != nil {
			return err
		}
	}
	return nil
}

// Enable enables a specific logger with its package name
func Enable(pkg string) error {
	l := create(pkg)
	enabledLoggers[pkg] = l
	return nil
}

// Disable a specific logger by its package name
func Disable(prefix string) {
	nop := zerolog.Nop()
	enabledLoggers[prefix] = &nop
}

func create(pkg string) *zerolog.Logger {
	pid := os.Getpid()
	zl := createLog(pkg, pid)
	l := zl.With().Str("pkg", pkg).Int("pid", pid).Logger()
	return &l
}

// New returns a new Logger
func New(pkg string) *Logger {
	pkgs = append(pkgs, pkg)
	nop := zerolog.Nop()
	enabledLoggers[pkg] = &nop
	logger := &Logger{pkg: pkg}
	return logger
}

func find(pkg string) *zerolog.Logger {
	l := enabledLoggers[pkg]
	return l
}

// Builder allows to contruct log step by step
type Builder struct {
	event *zerolog.Event
	l     *Logger
}

// Str add a string to the builder
func (b *Builder) Str(key, val string) *Builder {
	b.event = b.event.Str(key, val)
	return b
}

// Int adds an int to the builder
func (b *Builder) Int(key string, val int) *Builder {
	b.event = b.event.Int(key, val)
	return b
}

// Msg write the message with any fields stored
func (b *Builder) Msg(ctx context.Context, msg string) {
	b.event.Str("trace", getTrace(ctx)).Msg(msg)
}

// Build allocates a new Builder
func (l *Logger) Build() *Builder {
	return &Builder{l: l, event: enabledLoggers[l.pkg].Info()}
}

// BuildError allocates a new Builder with error level
func (l *Logger) BuildError() *Builder {
	return &Builder{l: l, event: enabledLoggers[l.pkg].Error()}
}

// Println prints in info level
func (l *Logger) Println(ctx context.Context, args ...interface{}) {
	zl := find(l.pkg)
	zl.Info().Str("trace", getTrace(ctx)).Msg(fmt.Sprint(args...))
}

// Printf prints in info level
func (l *Logger) Printf(ctx context.Context, format string, args ...interface{}) {
	zl := find(l.pkg)
	zl.Info().Str("trace", getTrace(ctx)).Msg(fmt.Sprintf(format, args...))
}

// Error prints in error level
func (l *Logger) Error(ctx context.Context, err error) {
	zl := find(l.pkg)
	zl.Error().Str("trace", getTrace(ctx)).Msg(err.Error())
}

// Panic prints in error levzel a stack trace
func (l *Logger) Panic(ctx context.Context, reason string) {
	zl := find(l.pkg)
	stack := debug.Stack()
	msg := reason + "\n" + string(stack)
	zl.Error().Str("trace", getTrace(ctx)).Bool("panic", true).Msg(msg)
}

func createLog(pkg string, pid int) *zerolog.Logger {
	zlog := zerolog.New(os.Stderr).With().Str("pkg", pkg).Int("pid", pid).Timestamp().Caller().Logger()
	if Mode == "" || Mode == "dev" {
		zlog = zlog.Output(zerolog.ConsoleWriter{Out: Out})
	} else {
		zlog = zlog.Output(Out)
	}
	return &zlog
}

func getTrace(ctx context.Context) string {
	if v, ok := ctx.Value("trace").(string); ok {
		return v
	}
	return ""
}
