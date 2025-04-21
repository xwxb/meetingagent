package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"meetingagent/models"
	"meetingagent/services"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/sse"
)

// --- Placeholder for Repository Dependency ---
// In a real app, this would be properly injected (e.g., via a handler struct)
var meetingRepo models.MeetingRepository

// SetMeetingRepository allows setting the repository (simple injection for now)
func SetMeetingRepository(repo models.MeetingRepository) {
	meetingRepo = repo
}

// CreateMeeting handles the creation of a new meeting from raw JSON content
func CreateMeeting(ctx context.Context, c *app.RequestContext) {
	if meetingRepo == nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "Repository not initialized"})
		return
	}

	// Get file content
	body, err := c.Body()
	if err != nil {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "Failed to read request body"})
		return
	}

	// Get the original filename from header
	fileName := string(c.GetHeader("X-File-Name"))
	if fileName == "" {
		fileName = "meeting_" + time.Now().Format("20060102150405")
	}

	currentTime := time.Now()
	meeting := &models.Meeting{
		Name:          fileName,
		AudioFilename: fileName,
		Transcript:    sql.NullString{String: string(body), Valid: true},
		UploadedAt:    currentTime,
		ModifiedAt:    currentTime,
	}

	// Try to create with original name
	newID, err := meetingRepo.CreateMeeting(meeting)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		// On name conflict, generate new name with timestamp and try again
		meeting.Name = "meeting_" + time.Now().Format("20060102150405")
		meeting.AudioFilename = meeting.Name
		newID, err = meetingRepo.CreateMeeting(meeting)
		if err != nil {
			c.JSON(consts.StatusInternalServerError, utils.H{"error": "Failed to create meeting record: " + err.Error()})
			return
		}
	} else if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "Failed to create meeting record: " + err.Error()})
		return
	}

	// Generate summary asynchronously
	go func(meetingID int64, transcript string) {
		sr, err := services.GetMeetingSummary(ctx, transcript)
		if err != nil {
			fmt.Printf("Error generating summary for meeting %d: %v\n", meetingID, err)
			return
		}

		jsonByte, marshalErr := json.Marshal(sr)
		if marshalErr != nil {
			fmt.Printf("Error marshalling summary response for meeting %d: %v\n", meetingID, marshalErr)
			return
		}
		jsonStr := string(jsonByte)

		// Update meeting with summary
		meeting.Summary = sql.NullString{String: jsonStr, Valid: true}
		meeting.ChatHistory = sql.NullString{String: jsonStr, Valid: true}
		meeting.ModifiedAt = time.Now()
		if updateErr := meetingRepo.UpdateMeeting(meetingID, meeting); updateErr != nil {
			fmt.Printf("Error updating meeting %d with summary: %v\n", meetingID, updateErr)
			return
		}
	}(newID, meeting.Transcript.String)

	response := models.PostMeetingResponse{
		ID: newID,
	}
	c.JSON(consts.StatusCreated, response)
}

// ListMeetings handles listing all meetings
func ListMeetings(ctx context.Context, c *app.RequestContext) {
	if meetingRepo == nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "Repository not initialized"})
		return
	}

	meetings, err := meetingRepo.ListMeetings()
	if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "Failed to retrieve meetings: " + err.Error()})
		return
	}

	response := models.GetMeetingsResponse{
		Meetings: meetings,
	}
	c.JSON(consts.StatusOK, response)
}

// GetMeetingSummary handles retrieving a meeting summary
func GetMeetingSummary(ctx context.Context, c *app.RequestContext) {
	if meetingRepo == nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "Repository not initialized"})
		return
	}

	meetingIDStr := c.Query("meeting_id")
	if meetingIDStr == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id is required"})
		return
	}

	// Convert meeting_id to int64
	var meetingID int64
	_, err := fmt.Sscanf(meetingIDStr, "%d", &meetingID)
	if err != nil {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "Invalid meeting_id format"})
		return
	}

	// Get meeting from repository
	meeting, err := meetingRepo.GetMeetingByID(meetingID)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "Failed to retrieve meeting: " + err.Error()})
		return
	}

	// If summary doesn't exist yet, generate it
	if !meeting.Summary.Valid || meeting.Summary.String == "" {
		c.JSON(consts.StatusOK, utils.H{
			"content": "The summary is still being generated. Please try again in a moment.",
		})
		return
	}

	// Parse tasks from JSON if available todo


	c.JSON(consts.StatusOK, meeting.Summary.String)
}

// HandleChat handles the SSE chat session
func HandleChat(ctx context.Context, c *app.RequestContext) {
	meetingID := c.Query("meeting_id")
	sessionID := c.Query("session_id")
	message := c.Query("message")

	if meetingID == "" || sessionID == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id and session_id are required"})
		return
	}

	if message == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "message is required"})
		return
	}

	fmt.Printf("meetingID: %s, sessionID: %s, message: %s\n", meetingID, sessionID, message)

	// Set SSE headers
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")

	// Create SSE stream
	stream := sse.NewStream(c)

	// TODO: Implement actual chat logic
	// This is a simple example that sends a message every second
	ticker := time.NewTicker(time.Millisecond * 100)
	stopChan := make(chan struct{})
	go func() {
		time.AfterFunc(time.Second, func() {
			ticker.Stop()
			close(stopChan)
		})
	}()

	msg := fmt.Sprintf("Fake sample chat message: %s\n", time.Now().Format(time.RFC3339))

	for {
		select {
		case <-ticker.C:
			res := models.ChatMessage{
				Data: msg,
			}

			data, err := json.Marshal(res)
			if err != nil {
				return
			}

			event := &sse.Event{
				Data: data,
			}

			if err := stream.Publish(event); err != nil {
				return
			}
		case <-stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}
