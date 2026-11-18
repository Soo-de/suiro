// Package api serves the HTTP/JSON API and the embedded UI. Handlers
// authenticate the caller, pass every result through a ScopeFilter
// (implemented by package scope) and translate to and from model types; they
// hold no analysis logic. Analysis runs on the whole model; the filter
// applies only to what a caller is shown.
package api
