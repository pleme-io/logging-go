package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// stderr is the os.Stderr writer, named so [outputWriter] reads cleanly.
var stderr io.Writer = os.Stderr

// Config is the typed, yaml-tagged knob surface for a logger (Law 3). It is the
// sub-struct a caller's shikumi-loaded root config embeds:
//
//	type Root struct {
//	    Logging logging.Config `yaml:"logging"`
//	    // …
//	}
//
// and hands to [FromConfig]. Per §3.5, [FromConfig] consumes this already-loaded
// struct and MUST NOT itself call shikumi.Load — config loading happens once, at
// main, via shikumi.For[Root]; every primitive only consumes its sub-struct.
//
// Precedence (args > env > file) is resolved by shikumi during the load, not
// here. The zero value is a valid config: it logs JSON to stdout at info level.
type Config struct {
	// Level is the minimum level name: debug/info/warn/error (case-insensitive,
	// via [ParseLevel]). Empty means info. Applied to a live [*slog.LevelVar],
	// so a shikumi reload of this field retargets verbosity via [SetLevel].
	Level string `yaml:"level" json:"level"`
	// Format selects the sink: json (default) / text / console / discard. An
	// unknown value falls back to json. console requires a console [SinkFunc]
	// installed via [WithSink] (logging-go/console); without one it degrades to
	// text so the zero-dep core stays buildable.
	Format string `yaml:"format" json:"format"`
	// AddSource toggles file:line source annotation on every record.
	AddSource bool `yaml:"addSource" json:"addSource"`
	// Output names the destination: "stdout" (default), "stderr", or "discard".
	// A file/socket sink is wired by the caller via [WithWriter] rather than a
	// path here, keeping the core free of filesystem-open policy. Unknown values
	// fall back to stdout.
	Output string `yaml:"output" json:"output"`
}

// fromConfigOptions converts a [Config] into the [Option]s [FromConfig] applies,
// so [FromConfig] and any caller wanting to layer extra options share one
// translation. Caller-supplied opts (passed after these) win by being applied
// last (last-option-wins for level/format/etc.).
func (c Config) toOptions() ([]Option, error) {
	var opts []Option

	if c.Level != "" {
		lvl, err := ParseLevel(c.Level)
		if err != nil {
			return nil, fmt.Errorf("logging: config: %w", err)
		}
		opts = append(opts, WithLevel(lvl))
	}
	if c.Format != "" {
		opts = append(opts, WithFormat(c.Format))
	}
	opts = append(opts, WithAddSource(c.AddSource))
	if w := outputWriter(c.Output); w != nil {
		opts = append(opts, WithWriter(w))
	}
	return opts, nil
}

// outputWriter resolves the Output name to an [io.Writer], or nil to leave the
// default (stdout) in place. "discard" maps to [io.Discard]; the
// [FormatDiscard] sink is the dedicated way to drop records, but routing the
// writer to discard is honoured too.
func outputWriter(name string) io.Writer {
	switch name {
	case "stderr":
		return stderr
	case "discard":
		return io.Discard
	default:
		return nil
	}
}

// FromConfig builds a [*slog.Logger] from an already-loaded [Config] (§3.5). It
// is the canonical config-consuming constructor: it takes the sub-struct and
// MUST NOT call shikumi.Load. Extra opts are applied after the config-derived
// ones, so a caller can layer non-yaml knobs (a [SinkFunc] for the console, a
// custom [FieldExtractor], redaction [Middleware]) on top:
//
//	log, err := logging.FromConfig(cfg.Logging,
//	    logging.WithSink(console.Sink(theme.Default())),
//	)
//
// It returns an error only for an invalid Level string; all other fields have
// safe fallbacks, so the zero Config yields the New default (JSON/stdout/info).
func FromConfig(cfg Config, opts ...Option) (*slog.Logger, error) {
	base, err := cfg.toOptions()
	if err != nil {
		return nil, err
	}
	return New(append(base, opts...)...), nil
}
