// Package cert is the connector certification suite. It exposes table-driven
// harnesses (RunLLM, RunTool) that any connector author can wire into a test
// to assert the connector honors the contract every Kave connector must meet
// before it ships: capability shape, route coverage, request preparation,
// auth strip/inject, streaming aggregation, usage parsing, tool-call
// extraction, policy/budget contracts, error tolerance, and golden wire shape.
//
// The suite is intentionally transport-agnostic — it does not boot a server
// or make network calls. It exercises the connector interfaces in
// core/connectors/runtime against caller-supplied fixtures. Add a new
// connector by writing a cert_test.go that constructs a Spec and calls
// cert.RunLLM(t, spec) (or RunTool).
//
// Set the env var KAVE_UPDATE_GOLDEN=1 to refresh golden files in place.
package cert
