// Package rpc defines the Collector and Renderer interfaces the server
// depends on, each with two implementations: a gRPC client and server over
// the generated code in internal/gen, and an in-process adapter for the all-
// in-one mode and tests. Callers depend on the interfaces, never on gRPC
// types.
package rpc
