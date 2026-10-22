// Package logger is a thin wrapper around log/slog that attaches a request ID
// to context-aware log calls.
//
// By default it logs through slog.Default(), which writes via the standard log
// package, so its lines share the output of existing log.Printf calls. Init
// installs a different handler for this package only.
//
//	ctx = logger.WithRequestID(ctx, id)
//	logger.InfoContext(ctx, "login: successful") // includes request_id=<id>
package logger
