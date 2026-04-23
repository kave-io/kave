// Package anthropic implements the Kave connector for the Anthropic Claude API.
//
// # Targeted API
//
// API version: 2023-06-01 (stable, passed via anthropic-version header on every request)
// Base URL: https://api.anthropic.com
//
// # Supported operations
//
//   - messages           — POST /v1/messages (non-streaming)
//   - messages.streaming — POST /v1/messages with stream:true (SSE)
//
// # Authentication
//
// Requires an API key passed as:
//
//	x-api-key: <key>
//
// Note: Anthropic does NOT use the Bearer scheme. The anthropic-version header
// is required on every request (value: "2023-06-01").
//
// # Streaming
//
// Streaming uses server-sent events. Content arrives in content_block_delta
// events. input_tokens is reported in message_start; output_tokens in
// message_delta (final event before message_stop).
//
// # Usage
//
//	client := anthropic.New(apiKey, 30*time.Second)
//	connector := anthropic.NewConnector(client)
//
// # Versioning note
//
// If Anthropic releases a new API version, update APIVersion in this package,
// the apiVersion constant in client.go, and the affected request/response types.
package anthropic
