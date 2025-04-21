package services

import (
	"context"
	"encoding/json"
	"fmt"
	"meetingagent/config"
	"meetingagent/models"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// GetMeetingSummary generates a summary for a meeting given its transcript
func GetMeetingSummary(ctx context.Context, transcript string) (*models.SummaryResponse, error) {
	if config.AppConfig == nil {
		return nil, fmt.Errorf("application config not initialized")
	}

	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  config.AppConfig.APIKey,
		BaseURL: config.AppConfig.BaseURL,
		Model:   config.AppConfig.Summary.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chat model: %v", err)
	}

	// Prepare messages for the LLM
	messages := []*schema.Message{
		config.AppConfig.GetSummarySystemMessage(),
		{
			Role:    schema.User,
			Content: transcript,
		},
	}

	// Generate summary
	response, err := cm.Generate(ctx, messages, model.WithTemperature(0.8))
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %v", err)
	}

	// log.Default().Printf("Response: %s", response.Content)

	// 这里做一个处理，如果第一行和最后一行包含 ``` 则去掉
	if len(response.Content) > 0 {
		if response.Content[0] == '`' && response.Content[len(response.Content)-1] == '`' {
			response.Content = response.Content[7 : len(response.Content)-3]
		}
	}
	// log.Default().Printf("Processed Response: %s", response.Content)

	// Parse the JSON response
	var summaryResponse models.SummaryResponse
	err = json.Unmarshal([]byte(response.Content), &summaryResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse summary response: %v", err)
	}

	return &summaryResponse, nil
}
