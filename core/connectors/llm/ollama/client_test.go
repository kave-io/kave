package ollama_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kave-io/kave/core/connectors/llm/ollama"
)

const testModel = "qwen2.5:0.5b"
const embedModel = "qwen3-embedding:4b"

func setupClient(t *testing.T) *ollama.Client {
	c := ollama.New("http://localhost:11434", 10*time.Second)
	if err := c.Ping(context.Background()); err != nil {
		t.Skipf("Skipping integration tests: Ollama not reachable: %v", err)
	}
	return c
}

func TestVRAMLifecycle(t *testing.T) {
	c := setupClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Ensure model is unloaded initially
	err := c.ForceUnload(ctx, testModel)
	if err != nil {
		t.Fatalf("Failed to reset VRAM state: %v", err)
	}

	// 2. Load Model
	err = c.LoadModel(ctx, testModel)
	if err != nil {
		t.Fatalf("Failed to load model: %v", err)
	}

	// 3. Verify it is running
	running, err := c.IsRunning(ctx, testModel)
	if err != nil || !running {
		t.Fatalf("Expected model to be running in VRAM, got running=%v, err=%v", running, err)
	}

	// 4. Force Unload
	err = c.ForceUnload(ctx, testModel)
	if err != nil {
		t.Fatalf("ForceUnload failed: %v", err)
	}

	// 5. Verify it is evicted
	running, err = c.IsRunning(ctx, testModel)
	if err != nil || running {
		t.Fatalf("Expected model to be evicted from VRAM, got running=%v", running)
	}
}

func TestSimpleChat(t *testing.T) {
	c := setupClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := ollama.ChatRequest{
		Model: testModel,
		Messages: []ollama.Message{
			{Role: ollama.RoleUser, Content: "Say exactly 'hello' and nothing else."},
		},
	}

	resp, err := c.ChatOneShot(ctx, req)
	if err != nil {
		t.Fatalf("ChatOneShot failed: %v", err)
	}

	if resp.Message.Content == "" {
		t.Error("Expected a response, got empty string")
	}
}

func TestEmbeddings(t *testing.T) {
	c := setupClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	texts := []string{"This is a test document", "Another document"}

	embeddings, err := c.Embed(ctx, embedModel, texts, ollama.PrefixNone)
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if len(embeddings) != 2 {
		t.Errorf("Expected 2 embeddings, got %d", len(embeddings))
	}
	if len(embeddings[0]) == 0 {
		t.Error("Expected embedding vector to have length > 0")
	}
}

func TestSlidingWindowNeverDropsSystemPrompt(t *testing.T) {
	c := setupClient(t)
	ctx := context.Background()

	session := c.NewSession(testModel, "You are a helpful assistant.", 4, nil)
	defer session.Close(ctx)

	// Send 6 messages — window is 4, so first 2 user turns should be dropped
	for i := range 6 {
		_, err := session.Send(ctx, fmt.Sprintf("message %d", i))
		if err != nil {
			t.Fatalf("Send %d failed: %v", i, err)
		}
	}

	windowed := session.WindowedHistory()

	// System prompt must always be first
	if windowed[0].Role != ollama.RoleSystem {
		t.Errorf("expected system prompt at position 0, got %s", windowed[0].Role)
	}

	// Window should be 4 turns + 1 system = 5 total
	if len(windowed) != 5 {
		t.Errorf("expected 5 messages in window (1 system + 4 history), got %d", len(windowed))
	}
}

func TestEmbedEvictAfter(t *testing.T) {
	c := setupClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := c.EmbedOne(ctx, embedModel, "test", ollama.PrefixNone, ollama.EvictAfter())
	if err != nil {
		t.Fatalf("EmbedOne failed: %v", err)
	}

	// Give the goroutine time to evict
	time.Sleep(500 * time.Millisecond)

	running, err := c.IsRunning(ctx, embedModel)
	if err != nil {
		t.Fatalf("IsRunning failed: %v", err)
	}
	if running {
		t.Error("expected model to be evicted after EvictAfter()")
	}
}
