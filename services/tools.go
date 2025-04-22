package services

import (
	"context"
	"encoding/json"
	"fmt"
	"meetingagent/config"
	"meetingagent/models"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// TaskAction represents a task action intent - kept here for Task Specialist logic
type TaskAction struct {
	MeetingID string `json:"meeting_id"`
	TaskIndex string `json:"task_index"`
	Status    string `json:"status"`
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
