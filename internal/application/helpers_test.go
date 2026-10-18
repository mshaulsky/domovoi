package application

import (
	"io"
	"log/slog"
)

// quiet is a logger that keeps test output clean.
var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))
