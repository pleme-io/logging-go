module github.com/pleme-io/logging-go/redact

go 1.25

require (
	github.com/pleme-io/logging-go v0.0.0
	github.com/samber/slog-formatter v1.3.0
)

require (
	github.com/samber/lo v1.53.0 // indirect
	github.com/samber/slog-common v0.21.0 // indirect
	github.com/samber/slog-multi v1.8.0 // indirect
	golang.org/x/text v0.22.0 // indirect
)

// Declarative redaction is a leaf module (Law 6): the dep-bearing
// samber/slog-formatter feature is quarantined here so the logging core stays
// zero-dep and offline-buildable. The logging-go replace is consumed from the
// local working tree until the upstream tag exists on the module proxy.
replace github.com/pleme-io/logging-go => ../
