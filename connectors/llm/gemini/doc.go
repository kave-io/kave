// Package gemini implements the Kave connector for the Google Gemini API.
//
// # Targeted API
//
// API version: v1beta (https://generativelanguage.googleapis.com/v1beta)
// Note: Google is stabilising the Gemini REST API. When v1 is promoted to stable,
// update APIVersion and the base URL in client.go.
//
// # Supported operations
//
//   - generateContent           — POST /v1beta/models/{model}:generateContent
//   - generateContent.streaming — POST /v1beta/models/{model}:streamGenerateContent?alt=sse
//
// The model identifier is passed per-call (e.g. "gemini-2.5-flash"), not at
// construction time, because Gemini embeds the model in the URL path.
//
// # Authentication
//
// Requires an API key passed as:
//
//	x-goog-api-key: <key>
//
// The key can also be passed as a ?key= query parameter, but the header is
// preferred to avoid accidental logging of the key in access logs.
//
// # Token usage
//
// Token counts are in usageMetadata: promptTokenCount, candidatesTokenCount,
// totalTokenCount. In streaming, the final SSE chunk carries the full counts.
//
// # Usage
//
//	client := gemini.New(apiKey, 30*time.Second)
//	connector := gemini.NewConnector(client)
//
// # Versioning note
//
// If Google promotes the API to v1 or changes the endpoint structure, update
// APIVersion, defaultBase in client.go, and the affected types in api.go.
package gemini
