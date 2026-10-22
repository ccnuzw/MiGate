// Package web serves the MiGate panel: the JSON API under /api/, client
// subscription links under /sub/, and the embedded WebUI.
//
// NewRouter builds the handler from Options (WithStore, WithAuth, ...). Every
// /api/ route is declared in routeTable with its auth and CSRF policy, and the
// auth and CSRF middleware enforce those policies.
package web
