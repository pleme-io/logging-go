// Package console is the gated, borealis-themed development/CLI sink for
// logging-go. It renders slog records as styled console lines — a coloured
// level glyph (comp.Glyph), the message, and aligned key/value attributes
// (comp.KV) — the zerolog ConsoleWriter / first-class dev console that the
// elevation plan calls table-stakes (§2.3, item 5).
//
// It is a LEAF sub-package in its own module (Law 8): the zero-dep logging core
// never imports borealis, so the happy path stays offline-buildable. Consumers
// opt in by wiring this sink into the core through [logging.WithSink]:
//
//	import (
//	    "github.com/pleme-io/logging-go"
//	    "github.com/pleme-io/logging-go/console"
//	    "github.com/pleme-io/borealis/theme"
//	)
//
//	log, _ := logging.FromConfig(cfg.Logging, logging.WithSink(console.Sink(theme.Default())))
//
// Because it is composed through [logging.WithSink], the borealis-rendered
// console is just another terminal sink in the same redact → inject → sample →
// sink pipeline — Law 2 composition over the slog.Handler interface, no bespoke
// owner type.
package console

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/pleme-io/borealis/comp"
	"github.com/pleme-io/borealis/theme"
	"github.com/pleme-io/logging-go"
)

// Sink returns a [logging.SinkFunc] that builds a borealis-themed console
// [slog.Handler] over the resolved writer and handler options. Pass it to
// [logging.WithSink] (or [logging.FromConfig]'s opts) to select the console
// rendering for the FormatConsole format. The level filter from opts.Level is
// honoured, so a live [logging.SetLevel] retargets the console too.
func Sink(t theme.Theme) logging.SinkFunc {
	return func(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		var leveler slog.Leveler = slog.LevelInfo
		if opts != nil && opts.Level != nil {
			leveler = opts.Level
		}
		return &Handler{theme: t, w: w, leveler: leveler}
	}
}

// Handler is a borealis-themed [slog.Handler] that renders records as styled
// console lines. It is constructed via [Sink]; the [logging.SinkFunc] seam is
// the supported entry point, but the type is exported so it can be embedded in
// a hand-built pipeline (e.g. a slog-multi Fanout) when needed.
type Handler struct {
	theme   theme.Theme
	w       io.Writer
	leveler slog.Leveler

	mu     sync.Mutex // guards writes so concurrent records don't interleave
	groups []string   // open group prefix path
	attrs  []slog.Attr // accumulated WithAttrs, group-qualified
}

// Enabled reports whether a record at level passes the configured level filter.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.leveler.Level()
}

// roleForLevel maps an slog level onto a borealis semantic [theme.Role], so the
// theme — not this handler — owns the colour decision (one place for colour).
func roleForLevel(level slog.Level) theme.Role {
	switch {
	case level >= slog.LevelError:
		return theme.Danger
	case level >= slog.LevelWarn:
		return theme.Warning
	case level >= slog.LevelInfo:
		return theme.Info
	default:
		return theme.Neutral // debug and below
	}
}

// Handle renders the record: "<glyph> <LEVEL> message    k=v …" with the level
// glyph coloured by [roleForLevel] and the attributes laid out by [comp.KV].
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	glyph := comp.Glyph(h.theme, roleForLevel(r.Level))

	var pairs []comp.Pair
	// Handler-level attrs first (already group-qualified), then record attrs.
	for _, a := range h.attrs {
		pairs = appendPair(pairs, "", a)
	}
	prefix := groupPrefix(h.groups)
	r.Attrs(func(a slog.Attr) bool {
		pairs = appendPair(pairs, prefix, a)
		return true
	})

	line := fmt.Sprintf("%s %-5s %s", glyph, levelLabel(r.Level), r.Message)
	if len(pairs) > 0 {
		line += "  " + comp.KV(h.theme, pairs)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, line+"\n")
	return err
}

// WithAttrs returns a derived handler carrying attrs, qualified by any open
// groups, matching slog handler semantics.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	prefix := groupPrefix(h.groups)
	next := h.clone()
	// Build group-qualified copies of the incoming attrs and append them.
	qualified := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		qualified = append(qualified, qualifyAttr(prefix, a))
	}
	next.attrs = append(next.attrs, qualified...)
	return next
}

// WithGroup returns a derived handler that opens the named group; an empty name
// is a no-op per the slog.Handler contract.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := h.clone()
	next.groups = append(append([]string{}, h.groups...), name)
	return next
}

// clone copies the mutable, non-mutex fields into a fresh handler so derived
// handlers never share backing slices with their parent.
func (h *Handler) clone() *Handler {
	return &Handler{
		theme:   h.theme,
		w:       h.w,
		leveler: h.leveler,
		groups:  append([]string{}, h.groups...),
		attrs:   append([]slog.Attr{}, h.attrs...),
	}
}

// levelLabel is the short upper-case level name shown after the glyph.
func levelLabel(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERROR"
	case l >= slog.LevelWarn:
		return "WARN"
	case l >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}

// groupPrefix joins open groups into a dotted prefix (e.g. "http.req.").
func groupPrefix(groups []string) string {
	if len(groups) == 0 {
		return ""
	}
	p := ""
	for _, g := range groups {
		p += g + "."
	}
	return p
}

// qualifyAttr prefixes an attr's key with the group path (flattening nested
// group values onto dotted keys for the single-line console rendering).
func qualifyAttr(prefix string, a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	return slog.Attr{Key: prefix + a.Key, Value: a.Value}
}

// appendPair flattens an attr into comp.Pair rows, expanding group-valued attrs
// onto dotted keys so structure survives the single-line console format.
func appendPair(pairs []comp.Pair, prefix string, a slog.Attr) []comp.Pair {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		gp := prefix + a.Key + "."
		if a.Key == "" {
			gp = prefix
		}
		for _, ga := range a.Value.Group() {
			pairs = appendPair(pairs, gp, ga)
		}
		return pairs
	}
	return append(pairs, comp.Pair{K: prefix + a.Key, V: a.Value.String()})
}
