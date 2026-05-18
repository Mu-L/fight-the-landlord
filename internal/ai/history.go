package ai

import (
	"sync"

	"github.com/openai/openai-go/v3"
)

type turnRecord struct {
	user      string
	assistant string
}

type History struct {
	mu       sync.Mutex
	turns    []turnRecord
	maxTurns int
}

func newHistory() *History {
	return &History{maxTurns: 8}
}

func (h *History) AddTurn(user, assistant string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.turns = append(h.turns, turnRecord{user: user, assistant: assistant})
}

// BuildMessages 构建 LLM 消息列表（系统 prompt + 历史记录 + 新用户消息）
func (h *History) BuildMessages(newUserMsg string) []openai.ChatCompletionMessageParamUnion {
	h.mu.Lock()
	defer h.mu.Unlock()

	start := max(len(h.turns)-h.maxTurns, 0)
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, 1+2*(len(h.turns)-start)+1)
	msgs = append(msgs, openai.SystemMessage(buildSystemPrompt()))

	for _, t := range h.turns[start:] {
		msgs = append(msgs,
			openai.UserMessage(t.user),
			openai.AssistantMessage(t.assistant),
		)
	}

	msgs = append(msgs, openai.UserMessage(newUserMsg))
	return msgs
}
