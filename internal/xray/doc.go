// Package xray builds Xray-core configuration from MiGate's inbounds,
// outbounds and routing rules (BuildConfig and its variants), generates
// REALITY key pairs, and reads per-user and per-inbound traffic counters from
// Xray's StatsService over gRPC or via `xray api statsquery`.
package xray
