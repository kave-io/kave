// Package postgres contains the Postgres-only persistence foundation for the
// Kave V2 kernel.
//
// It is deliberately isolated from the V1 aggregate stores. Callers enter the
// database through ScopedRunner, which installs account and namespace identity
// as transaction-local Postgres settings before any application query runs.
package postgres
