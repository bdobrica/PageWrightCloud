package clients

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

type LLMClient struct {
	client *openai.Client
}

func NewLLMClient(apiKey, baseURL string) *LLMClient {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	return &LLMClient{
		client: openai.NewClientWithConfig(config),
	}
}

// EvaluateRequest evaluates if the user request is clear or needs clarification
func (c *LLMClient) EvaluateRequest(userMessage string) (*EvaluationResponse, error) {
	template := fmt.Sprintf(`You are a website build assistant. A user wants to make changes to their website.

User request: %s

Your task: Determine if this request is clear and actionable, or if you need more information.

If the request is CLEAR and you understand what changes to make, respond with:
CLEAR: <brief summary of what you will do>

If the request is UNCLEAR or you need more information, respond with:
UNCLEAR: <specific question to ask the user>

Examples:
- "Change the hero title to 'Welcome'" → CLEAR: I will update the hero section title text to "Welcome"
- "Make it look better" → UNCLEAR: What specific aspect would you like me to improve? (colors, layout, typography, images?)
- "Add a contact form" → CLEAR: I will add a contact form to the site with name, email, and message fields

Your response:`, userMessage)

	resp, err := c.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: template,
				},
			},
			MaxTokens:   200,
			Temperature: 0.3,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to evaluate request: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	content := resp.Choices[0].Message.Content

	// Parse response
	if len(content) > 6 && content[:6] == "CLEAR:" {
		return &EvaluationResponse{
			IsClear: true,
			Summary: content[7:],
		}, nil
	} else if len(content) > 8 && content[:8] == "UNCLEAR:" {
		return &EvaluationResponse{
			IsClear:  false,
			Question: content[9:],
		}, nil
	}

	// Fallback: assume unclear
	return &EvaluationResponse{
		IsClear:  false,
		Question: "Could you please provide more details about what you'd like to change?",
	}, nil
}

// GenerateJobInstructions generates detailed instructions for the worker
func (c *LLMClient) GenerateJobInstructions(userMessage, clarification string) (string, error) {
	var template string

	if clarification != "" {
		template = fmt.Sprintf(`You are a website build assistant. Generate detailed, actionable instructions for a code execution agent.

Original user request: %s
User clarification: %s

Generate specific instructions that include:
1. Which files to modify (e.g., content/index.md, theme/style.css)
2. Exact changes to make
3. Any validation steps

Format your response as clear, step-by-step instructions that a code agent can follow.

Instructions:`, userMessage, clarification)
	} else {
		template = fmt.Sprintf(`You are a website build assistant. Generate detailed, actionable instructions for a code execution agent.

User request: %s

Generate specific instructions that include:
1. Which files to modify (e.g., content/index.md, theme/style.css)
2. Exact changes to make
3. Any validation steps

Format your response as clear, step-by-step instructions that a code agent can follow.

Instructions:`, userMessage)
	}

	resp, err := c.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: template,
				},
			},
			MaxTokens:   500,
			Temperature: 0.5,
		},
	)

	if err != nil {
		return "", fmt.Errorf("failed to generate instructions: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from LLM")
	}

	return resp.Choices[0].Message.Content, nil
}

type EvaluationResponse struct {
	IsClear  bool
	Summary  string // if clear
	Question string // if unclear
}
