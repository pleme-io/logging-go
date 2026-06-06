;; caixa.lisp — the single source of truth for logging-go's kind + ecosystem.
;;
;; Consumed by `pleme-doc-gen` for the SDLC pipeline (flake.nix +
;; .pleme-io-release.toml + auto-release workflow + nix module trio).
;; Re-emit the generated surface with:
;;   pleme-doc-gen caixa --source caixa.lisp --out . --force
;;
;; NOTE: the authored Go source + go.mod are NOT regenerated. The render
;; adds release scaffolding only; go.mod is protected across re-render.

(defcaixa logging-go
  :kind         :Biblioteca
  :ecosystem    :go

  :package      { :name        "logging-go"
                  :version     "0.1.0"
                  :license     "MIT"
                  :description "pleme-io's structured-logging convention for Go — the slog-based counterpart to the Rust tracing + tracing-subscriber stack."
                  :module-path "github.com/pleme-io/logging-go"
                  :repository  "https://github.com/pleme-io/logging-go"
                  :homepage    "https://github.com/pleme-io/logging-go"
                  :go-version  "1.25" }

  :supports     { :go ">=1.25" }

  :ci-config    { :bump    { :default-type "patch" }
                  :publish { :no-verify false } }

  :workflows    [ :auto-release ]
  :stacks       [ ]
  :depends-on   [ ]
  :exposes      [ :go-module ]
  :publish-to-git true)
