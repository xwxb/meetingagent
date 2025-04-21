package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"meetingagent/models"
	"meetingagent/services"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/sse"
)

// GetMeetingTasks handles retrieving tasks for a meeting
func GetMeetingTasks(ctx context.Context, c *app.RequestContext) {
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

	if !meeting.TasksJSON.Valid || meeting.TasksJSON.String == "" {
		c.JSON(consts.StatusOK, utils.H{
			"tasks":            []string{},
			"tasks_status_num": 0,
		})
		return
	}

	// Parse tasks from JSON
	var tasks []string
	if err := json.Unmarshal([]byte(meeting.TasksJSON.String), &tasks); err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "Failed to parse tasks: " + err.Error()})
		return
	}

	c.JSON(consts.StatusOK, utils.H{
		"tasks":            tasks,
		"tasks_status_num": meeting.TasksStatusNum,
	})
}

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
		// Parse the summary response
		var summaryResp models.SummaryResponse
		if err := json.Unmarshal(jsonByte, &summaryResp); err != nil {
			fmt.Printf("Error unmarshalling summary response for meeting %d: %v\n", meetingID, err)
			return
		}

		// Store summary text and tasks separately
		meeting.SummaryText = sql.NullString{String: summaryResp.Summary, Valid: true}

		// Convert tasks array to JSON string
		tasksJSON, err := json.Marshal(summaryResp.Tasks)
		if err != nil {
			fmt.Printf("Error marshalling tasks for meeting %d: %v\n", meetingID, err)
			return
		}
		meeting.TasksJSON = sql.NullString{String: string(tasksJSON), Valid: true}

		// Initialize tasks_status_num as 0
		meeting.TasksStatusNum = 0

		meeting.ChatHistory = sql.NullString{String: string(jsonByte), Valid: true}
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
	if !meeting.SummaryText.Valid || meeting.SummaryText.String == "" {
		c.JSON(consts.StatusOK, utils.H{
			"content": "The summary is still being generated. Please try again in a moment.",
		})
		return
	}

	// Parse tasks from JSON if available todo

	// Construct response JSON
	response := struct {
		SummaryText    string   `json:"summary"`
		Tasks          []string `json:"tasks"`
		TasksStatusNum int64    `json:"tasks_status_num"`
	}{
		SummaryText:    meeting.SummaryText.String,
		TasksStatusNum: meeting.TasksStatusNum,
	}

	// Parse tasks from JSON
	if meeting.TasksJSON.Valid {
		var tasks []string
		if err := json.Unmarshal([]byte(meeting.TasksJSON.String), &tasks); err == nil {
			response.Tasks = tasks
		}
	}

	c.JSON(consts.StatusOK, response)
}

// HandleChat handles the SSE chat session using real-time LLM interaction via multi-agent
func HandleChat(ctx context.Context, c *app.RequestContext) {
	meetingIDStr := c.Query("meeting_id")
	sessionID := c.Query("session_id")
	userMessage := c.Query("message")

	if meetingIDStr == "" || sessionID == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "meeting_id and session_id are required"})
		return
	}
	if userMessage == "" {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "message is required"})
		return
	}

	meetingID, err := strconv.ParseInt(meetingIDStr, 10, 64)
	if err != nil {
		c.JSON(consts.StatusBadRequest, utils.H{"error": "Invalid meeting_id format"})
		return
	}

	if meetingRepo == nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "Repository not initialized"})
		return
	}

	// Fetch meeting data (optional, for future context use)
	meetingInfo, err := meetingRepo.GetMeetingByID(meetingID)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, utils.H{"error": "Failed to retrieve meeting: " + err.Error()})
		return
	}

	// Set SSE headers
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("Access-Control-Allow-Origin", "*")

	sseStream := sse.NewStream(c)

	// Prepare user message for multi-agent
	msgs := []*schema.Message{
		{
			Role:    schema.System,
			Content: "Info: meetingId=" + strconv.FormatInt(meetingID, 10),
		},
		{
			Role:    schema.User,
			Content: "会议纪要：\n" + meetingInfo.Transcript.String,
		},
		{
			Role:    schema.User,
			Content: "会议总结：\n" + meetingInfo.SummaryText.String,
		},
		{
			Role:    schema.User,
			Content: userMessage,
		},
	}
	// Use global HostMAt from services package
	hostMA := services.HostMA
	if hostMA == nil {
		sseStream.Publish(&sse.Event{Event: "error", Data: []byte(`{"error": "Multi-agent not initialized"}`)})
		return
	}

	// Start streaming response from multi-agent
	out, err := hostMA.Stream(ctx, msgs)
	if err != nil {
		log.Printf("Failed to start multi-agent stream: %v", err)
		sseStream.Publish(&sse.Event{Event: "error", Data: []byte(`{"error": "Failed to start chat stream"}`)})
		return
	}
	defer out.Close()

	// Goroutine to pipe multi-agent stream to SSE stream
	go func() {
		defer out.Close()
		for {
			select {
			case <-ctx.Done():
				log.Println("Client disconnected, closing stream.")
				return
			default:
				chunk, err := out.Recv()
				if err != nil {
					if err == io.EOF {
						log.Println("Multi-agent stream finished.")
					} else {
						log.Printf("Error receiving chunk from multi-agent: %v", err)
						sseStream.Publish(&sse.Event{Event: "error", Data: []byte(`{"error": "Error receiving data from chat service"}`)})
					}
					return
				}
				res := models.ChatMessage{
					Data: chunk.Content,
				}
				jsonData, marshalErr := json.Marshal(res)
				if marshalErr != nil {
					log.Printf("Error marshalling SSE data: %v", marshalErr)
					continue
				}
				event := &sse.Event{
					Data: jsonData,
				}
				if pubErr := sseStream.Publish(event); pubErr != nil {
					log.Printf("Error publishing SSE event: %v. Client likely disconnected.", pubErr)
					return
				}
			}
		}
	}()

	<-ctx.Done()
	log.Println("HandleChat request context finished.")
}
