package ollama

import (
	"context"
	"fmt"
	"sync"
)

// Session manages a long-lived interactive session with a single model.
type Session struct {
	client       *Client
	model        string
	options      *ModelOptions
	mu           sync.Mutex
	systemPrompt string
	history      []Message // only user/assistant turns, never the system message
	maxMessages  int       // sliding window size (pairs of user+assistant)
}

func (c *Client) NewSession(model string, systemPrompt string, maxHistory int, opts *ModelOptions) *Session {
	if maxHistory <= 0 {
		maxHistory = 20
	}
	return &Session{
		client:       c,
		model:        model,
		options:      opts,
		systemPrompt: systemPrompt,
		history:      make([]Message, 0),
		maxMessages:  maxHistory,
	}
}

func (s *Session) Preload(ctx context.Context) error {
	return s.client.LoadModel(ctx, s.model)
}

// buildPayload constructs the message slice to send to Ollama.
// Must be called with s.mu held.
func (s *Session) buildPayload() []Message {
	// Window over history — take last maxMessages turns
	start := 0
	if len(s.history) > s.maxMessages {
		start = len(s.history) - s.maxMessages
	}
	windowed := s.history[start:]

	// System prompt always anchored at position 0
	payload := make([]Message, 0, len(windowed)+1)
	if s.systemPrompt != "" {
		payload = append(payload, Message{Role: RoleSystem, Content: s.systemPrompt})
	}
	payload = append(payload, windowed...)
	return payload
}

func (s *Session) Send(ctx context.Context, content string) (string, error) {
	s.mu.Lock()
	s.history = append(s.history, Message{Role: RoleUser, Content: content})
	payload := s.buildPayload() // build while locked — atomic snapshot
	s.mu.Unlock()

	resp, err := s.client.Chat(ctx, ChatRequest{
		Model:     s.model,
		Messages:  payload,
		KeepAlive: KeepAliveForever,
		Options:   s.options,
	})
	if err != nil {
		s.mu.Lock()
		s.history = s.history[:len(s.history)-1] // rollback user message
		s.mu.Unlock()
		return "", fmt.Errorf("session: send: %w", err)
	}

	s.mu.Lock()
	s.history = append(s.history, resp.Message)
	s.mu.Unlock()

	return resp.Message.Content, nil
}

// SendStreamChan returns a channel that streams the response token by token.
func (s *Session) SendStreamChan(ctx context.Context, content string) (<-chan StreamChunk, error) {
	s.mu.Lock()
	s.history = append(s.history, Message{Role: RoleUser, Content: content})
	payload := s.buildPayload() // atomic snapshot while locked
	s.mu.Unlock()

	clientChan, err := s.client.ChatStreamChan(ctx, ChatRequest{
		Model:     s.model,
		Messages:  payload,
		KeepAlive: KeepAliveForever,
		Options:   s.options,
	})
	if err != nil {
		s.mu.Lock()
		s.history = s.history[:len(s.history)-1] // rollback user message
		s.mu.Unlock()
		return nil, fmt.Errorf("session: stream: %w", err)
	}

	outChan := make(chan StreamChunk)
	go func() {
		defer close(outChan)
		var fullContent string
		for chunk := range clientChan {
			if chunk.Error != nil {
				// Rollback the user message on stream error
				s.mu.Lock()
				s.history = s.history[:len(s.history)-1]
				s.mu.Unlock()
				outChan <- chunk
				return
			}
			fullContent += chunk.Content
			outChan <- chunk
		}
		// Stream completed — commit assistant message
		s.mu.Lock()
		s.history = append(s.history, Message{Role: RoleAssistant, Content: fullContent})
		s.mu.Unlock()
	}()

	return outChan, nil
}

// History returns a snapshot of the full history including system prompt.
func (s *Session) History() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, 0, len(s.history)+1)
	if s.systemPrompt != "" {
		out = append(out, Message{Role: RoleSystem, Content: s.systemPrompt})
	}
	out = append(out, s.history...)
	return out
}

// WindowedHistory returns only what would be sent to Ollama right now.
func (s *Session) WindowedHistory() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buildPayload()
}

// Reset clears conversation history but preserves the system prompt.
func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = s.history[:0]
}

// Close unloads the model from VRAM.
func (s *Session) Close(ctx context.Context) error {
	return s.client.ForceUnload(ctx, s.model)
}
