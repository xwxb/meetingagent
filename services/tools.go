package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"meetingagent/config"
	"meetingagent/models"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func Init() {
	bgCtx := context.Background()
	if err := initSummaryChatModel(bgCtx); err != nil {
		log.Fatalf("failed to init SummaryChatModel: %v", err)
	}
	log.Printf("✔ ChatModels initialized")
}

var SummaryChatModel *ark.ChatModel

// var ChatAgentModel *ark.ChatModel

func initSummaryChatModel(ctx context.Context) error {
	if config.AppConfig == nil {
		return fmt.Errorf("application config not initialized")
	}

	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  config.AppConfig.APIKey,
		BaseURL: config.AppConfig.BaseURL,
		Model:   config.AppConfig.Summary.Model,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize chat model: %v", err)
	}

	SummaryChatModel = cm
	return nil
}

// GetMeetingSummary generates a summary for a meeting given its transcript
func GetMeetingSummary(ctx context.Context, transcript string) (*models.SummaryResponse, error) {
	if SummaryChatModel == nil {
		return nil, fmt.Errorf("summary chat model not initialized")
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
	response, err := SummaryChatModel.Generate(ctx, messages, model.WithTemperature(0.8))
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %v", err)
	}

	// Process response content
	if len(response.Content) > 0 {
		if response.Content[0] == '`' && response.Content[len(response.Content)-1] == '`' {
			response.Content = response.Content[7 : len(response.Content)-3]
		}
	}

	// Parse the JSON response
	var summaryResponse models.SummaryResponse
	err = json.Unmarshal([]byte(response.Content), &summaryResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse summary response: %v", err)
	}

	return &summaryResponse, nil
}
