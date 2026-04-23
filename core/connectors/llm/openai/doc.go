// Package openai implements the Kave connector for the OpenAI API.
//
// # Targeted API
//
// API version: v1 (https://api.openai.com/v1)
// Connector targets the stable v1 API with no versioning header required.
//
// # Supported operations
//
//   - chat.completions       — POST /v1/chat/completions (non-streaming)
//   - chat.completions.stream — POST /v1/chat/completions (SSE streaming)
//   - embeddings             — POST /v1/embeddings
//
// # Authentication
//
// Requires an API key passed as:
//
//	Authorization: Bearer <key>
//
// Optional org/project scoping via OpenAI-Organization and OpenAI-Project headers.
//
// # Usage
//
//	client := openai.New(apiKey, 30*time.Second)
//	connector := openai.NewConnector(client)
//
// # Versioning note
//
// If OpenAI introduces breaking changes to the API shape, update APIVersion,
// the request/response types in api.go, and bump the constant below.
package openai
