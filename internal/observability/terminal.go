package observability

import (
	"os"

	"github.com/mattn/go-isatty"
)

// isTerminal reports whether stdout is an interactive terminal, used to pick a
// human-readable (text) slog handler in development vs JSON in production.
func isTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}
