// Package redact is the gated, declarative redaction middleware for logging-go.
// It builds on samber/slog-formatter's FormatByKey / PIIFormatter — the sibling
// to the already-adopted slog-multi / slog-sampling ecosystem — rather than a
// hand-rolled ReplaceAttr, so redaction is declared by attribute key and
// composes as ordinary [logging.Middleware] in the redact → inject → sample →
// sink pipeline (Law 2). It is the mandatory secret-leak guard the elevation
// plan calls for (§2.3, item 6).
//
// It is a leaf sub-package in its own module (Law 6): the dep-bearing redaction
// feature lives here, quarantined, so the logging core stays zero-dep and
// offline-buildable. Pair it with [logging.SecretString] (the LogValuer in the
// core that redacts a value wherever it travels): SecretString protects a typed
// value, redact protects by key at the sink — use both.
//
//	mw := redact.Keys("token", "password", "api_key")  // → "[REDACTED]"
//	pii := redact.PII("user")                            // slog-formatter PII mask
//	log, _ := logging.FromConfig(cfg.Logging, logging.WithMiddleware(mw, pii))
//
// Because the redaction middleware is listed before the inject stage by
// [logging.WithMiddleware], it sees and rewrites records before context fields
// are injected — the correct position to scrub call-site secrets.
package redact

import (
	"log/slog"

	slogformatter "github.com/samber/slog-formatter"

	"github.com/pleme-io/logging-go"
)

// Keys returns a [logging.Middleware] that replaces the value of every attribute
// whose key matches one of keys with [logging.Redacted] ("[REDACTED]") — the
// same stable marker [logging.SecretString] uses, so aggregators see one token.
// Declarative and key-based; no per-call-site discipline required.
func Keys(keys ...string) logging.Middleware {
	formatters := make([]slogformatter.Formatter, 0, len(keys))
	redacted := slog.StringValue(logging.Redacted)
	for _, k := range keys {
		formatters = append(formatters, slogformatter.FormatByKey(k,
			func(slog.Value) slog.Value { return redacted },
		))
	}
	return slogformatter.NewFormatterHandler(formatters...)
}

// PII returns a [logging.Middleware] that masks the value(s) under each given key
// using slog-formatter's [slogformatter.PIIFormatter] — emails, addresses, and
// free-form fields become "********" while stable IDs are kept, matching the
// upstream PII convention. Use it for structured user records where partial
// redaction (keep IDs, mask the rest) is wanted.
func PII(keys ...string) logging.Middleware {
	formatters := make([]slogformatter.Formatter, 0, len(keys))
	for _, k := range keys {
		formatters = append(formatters, slogformatter.PIIFormatter(k))
	}
	return slogformatter.NewFormatterHandler(formatters...)
}

// FormatKey returns a [logging.Middleware] that rewrites the value under key via
// a custom formatter — the escape hatch for partial masking (e.g. show the last
// four digits) declared the same slog-formatter way as [Keys]/[PII], not as a
// bespoke ReplaceAttr.
func FormatKey(key string, formatter func(slog.Value) slog.Value) logging.Middleware {
	return slogformatter.NewFormatterHandler(slogformatter.FormatByKey(key, formatter))
}
