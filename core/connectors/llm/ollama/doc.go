// Package ollama implements the Kave connector for Ollama (local LLM runtime).
//
// # Targeted API
//
// API version: 0.6.x (https://github.com/ollama/ollama/blob/main/docs/api.md)
// Base URL: http://localhost:11434 (default; configurable at construction time)
//
// # Supported operations
//
//   - chat           — POST /api/chat (non-streaming and streaming via ChatStreamChan)
//   - generate       — POST /api/generate (non-streaming and streaming via GenerateStreamChan)
//   - embed          — POST /api/embed (batch embeddings)
//
// Model management (load, unload, list) is handled by the Client directly and
// does not go through the intercept pipeline.
//
// # Authentication
//
// None required for local Ollama. If running behind a reverse proxy with auth,
// set the Authorization header on the underlying *http.Client transport.
//
// # Session management
//
// Client.NewSession provides a stateful multi-turn chat with a sliding history
// window and thread-safe access. The system prompt is always anchored at
// position 0 regardless of windowing.
//
// # Usage
//
//	client := ollama.New("http://localhost:11434", 30*time.Second)
//	connector := ollama.NewConnector(client)
//
// # Versioning note
//
// Ollama does not use a versioned API path. If the request/response shapes
// change in a future release, update APIVersion and the affected types in api.go.
package ollama
