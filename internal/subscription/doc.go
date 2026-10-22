// Package subscription imports outbound nodes from external subscription URLs.
// It fetches a subscription with an HTTP client that refuses private-network
// targets, decodes the body into share links (ParseBody, ParseLinks), and
// turns them into stable outbound records (Materialize).
//
// Client subscription links served at /sub/{token} are built in package web.
package subscription
