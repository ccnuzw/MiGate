// Package db is MiGate's SQLite persistence layer (modernc.org/sqlite, no
// CGO). Open creates or migrates the schema in schema.go and returns a Store
// whose methods cover inbounds, clients, outbounds, outbound subscriptions,
// routing rules, certificates, sessions, login rate limits and traffic data.
package db
