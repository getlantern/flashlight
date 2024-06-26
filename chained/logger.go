package chained

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/getlantern/golog"
)

// slogHandler is a Handler that implements the slog.Handler interface
// and writes log records to a golog.Logger.
type slogHandler struct {
	logger   golog.Logger
	minLevel slog.Level
	opts     slog.HandlerOptions
	attrs    string
	groups   []string
}

func newLogHandler(logger golog.Logger) *slogHandler {
	return &slogHandler{logger: logger}
}

// Enabled reports whether the handler handles records at the given level.
// The handler ignores records whose level is lower.
// It is called early, before any arguments are processed,
// to save effort if the log event should be discarded.
// If called from a Logger method, the first argument is the context
// passed to that method, or context.Background() if nil was passed
// or the method does not take a context.
// The context is passed so Enabled can use its values
// to make a decision.
func (h *slogHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelDebug
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

// Handle handles the Record.
// It will only be called when Enabled returns true.
// The Context argument is as for Enabled.
// It is present solely to provide Handlers access to the context's values.
// Canceling the context should not affect record processing.
// (Among other things, log messages may be necessary to debug a
// cancellation-related problem.)
//
// Handle methods that produce output should observe the following rules:
//   - If r.Time is the zero time, ignore the time.
//   - If r.PC is zero, ignore it.
//   - Attr's values should be resolved.
//   - If an Attr's key and value are both the zero value, ignore the Attr.
//     This can be tested with attr.Equal(Attr{}).
//   - If a group's key is empty, inline the group's Attrs.
//   - If a group has no Attrs (even if it has a non-empty key),
//     ignore it.
func (h *slogHandler) Handle(ctx context.Context, record slog.Record) error {
	if !h.Enabled(ctx, record.Level) {
		return nil
	}

	messageBuilder := new(strings.Builder)
	messageBuilder.WriteString(record.Message)
	messageBuilder.WriteString(" ")
	record.Attrs(func(attr slog.Attr) bool {
		messageBuilder.WriteString(attr.Key)
		messageBuilder.WriteString("=")
		messageBuilder.WriteString(attr.Value.String())
		messageBuilder.WriteString(" ")
		return true
	})

	messageBuilder.WriteString(h.attrs)
	message := messageBuilder.String()

	switch record.Level {
	case slog.LevelDebug, slog.LevelInfo, slog.LevelWarn:
		h.logger.Debug(message)
	case slog.LevelError:
		err := h.logger.Error(message)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported log level: %v", record.Level)
	}
	return nil
}

// WithAttrs returns a new Handler whose attributes consist of
// both the receiver's attributes and the arguments.
// The Handler owns the slice: it may retain, modify or discard it.
func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.attrs = parseAttrs(attrs)
	return h
}

func parseAttrs(attrs []slog.Attr) string {
	attrsBuilder := new(strings.Builder)
	for _, attr := range attrs {
		attrsBuilder.WriteString(attr.Key)
		attrsBuilder.WriteString(":")
		switch attr.Value.Kind() {
		case slog.KindString, slog.KindAny:
			attrsBuilder.WriteString(attr.Value.String())
		case slog.KindInt64:
			fmt.Fprintf(attrsBuilder, "%d", attr.Value.Int64())
		case slog.KindUint64:
			fmt.Fprintf(attrsBuilder, "%d", attr.Value.Uint64())
		case slog.KindFloat64:
			fmt.Fprintf(attrsBuilder, "%f", attr.Value.Float64())
		case slog.KindBool:
			fmt.Fprintf(attrsBuilder, "%t", attr.Value.Bool())
		case slog.KindTime:
			attrsBuilder.WriteString(attr.Value.Time().String())
		case slog.KindDuration:
			attrsBuilder.WriteString(attr.Value.Duration().String())
		case slog.KindLogValuer:
			attrsBuilder.WriteString(attr.Value.LogValuer().LogValue().String())
		case slog.KindGroup:
			attrsBuilder.WriteString("{")
			attrsBuilder.WriteString(parseAttrs(attr.Value.Group()))
			attrsBuilder.WriteString("}")
		}
		attrsBuilder.WriteString(" ")
	}
	return attrsBuilder.String()
}

// WithGroup returns a new Handler with the given group appended to
// the receiver's existing groups.
// The keys of all subsequent attributes, whether added by With or in a
// Record, should be qualified by the sequence of group names.
//
// How this qualification happens is up to the Handler, so long as
// this Handler's attribute keys differ from those of another Handler
// with a different sequence of group names.
//
// A Handler should treat WithGroup as starting a Group of Attrs that ends
// at the end of the log event. That is,
//
//	logger.WithGroup("s").LogAttrs(ctx, level, msg, slog.Int("a", 1), slog.Int("b", 2))
//
// should behave like
//
//	logger.LogAttrs(ctx, level, msg, slog.Group("s", slog.Int("a", 1), slog.Int("b", 2)))
//
// If the name is empty, WithGroup returns the receiver.
func (h *slogHandler) WithGroup(name string) slog.Handler {
	// TODO: Implement WithGroup
	return h
}
