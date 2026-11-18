// Package rpc defines the Collector and Renderer interfaces the server and
// git-collector depend on, each with two implementations: a gRPC client and
// server over the generated code in internal/gen (contract in
// proto/suiro/v1), and an in-process adapter for the all-in-one mode and
// tests. Callers depend on the interfaces, never on gRPC types; the types in
// these signatures are model types or plain structs defined here.
package rpc
