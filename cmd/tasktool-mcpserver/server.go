package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func main() {
	// Open SQLite database
	var err error
	db, err = sql.Open("sqlite3", "./meetings.db")
	if err != nil {
		fmt.Printf("Failed to open database: %v\n", err)
		return
	}
	defer db.Close()

	// Create MCP server
	s := server.NewMCPServer(
		"Meeting Task Manager",
		"1.0.0",
	)

	// Create update task status tool
	updateTaskTool := mcp.NewTool("update_task_status",
		mcp.WithDescription("Update the completion status of a meeting task"),
		mcp.WithString("meeting_id",
			mcp.Required(),
			mcp.Description("ID of the meeting"),
		),
		mcp.WithString("task_index",
			mcp.Required(),
			mcp.Description("Index of the task to update (0-based)"),
		),
		mcp.WithString("status",
			mcp.Required(),
			mcp.Description("Task completion status (true/false)"),
		),
	)

	// Add tool handler
	s.AddTool(updateTaskTool, updateTaskStatusHandler)

	// Start the stdio server
	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

func updateTaskStatusHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract and convert parameters
	meetingIDStr, ok := request.Params.Arguments["meeting_id"].(string)
	if !ok {
		return nil, errors.New("meeting_id must be a string")
	}
	meetingID, err := strconv.ParseInt(meetingIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid meeting_id %v format: %v", meetingID, err)
	}

	taskIndexStr, ok := request.Params.Arguments["task_index"].(string)
	if !ok {
		return nil, errors.New("task_index must be a string")
	}
	taskIndex, err := strconv.Atoi(taskIndexStr)
	if err != nil {
		return nil, errors.New("invalid task_index format")
	}

	statusStr, ok := request.Params.Arguments["status"].(string)
	if !ok {
		return nil, errors.New("status must be a string")
	}
	status := statusStr == "true"

	// Get current meeting data
	var tasksJSON sql.NullString
	var statusNum int64
	err = db.QueryRow("SELECT tasks_json, tasks_status_num FROM meetings WHERE id = ?", meetingID).Scan(&tasksJSON, &statusNum)
	if err == sql.ErrNoRows {
		return nil, errors.New("meeting not found")
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %v", err)
	}

	// Verify task index is valid
	var tasks []string
	if err := json.Unmarshal([]byte(tasksJSON.String), &tasks); err != nil {
		return nil, fmt.Errorf("invalid tasks JSON: %v", err)
	}
	if taskIndex < 0 || taskIndex >= len(tasks) {
		return nil, errors.New("invalid task index")
	}

	// Update task status using binary flags
	if status {
		// Set bit to 1
		statusNum |= (1 << taskIndex)
	} else {
		// Set bit to 0
		statusNum &= ^(1 << taskIndex)
	}

	// Update database
	_, err = db.Exec("UPDATE meetings SET tasks_status_num = ? WHERE id = ?", statusNum, meetingID)
	if err != nil {
		return nil, fmt.Errorf("failed to update task status: %v", err)
	}

	response := fmt.Sprintf("Updated task %d status to %v for meeting %d. New status_num: %d",
		taskIndex, status, meetingID, statusNum)

	return mcp.NewToolResultText(response), nil
}
