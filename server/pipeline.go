package main

import (
	"github.com/kave-io/kave/core/intercept"
)

// NewPipeline builds the Kave intercept pipeline from a list of interceptors.
// Typical order: auth → cost → trace
//
// auth: Before checks if action is allowed | After is no-op
// cost: Before checks budget | After records token usage
// trace: Before starts span | After ends span
//
// This is the single pipeline used by all connectors and the HTTP proxy.
func NewPipeline(interceptors ...intercept.Interceptor) *intercept.Pipeline {
	return intercept.NewPipeline().Chain(interceptors...)
}
