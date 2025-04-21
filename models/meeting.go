package models

import (
	"database/sql"
	"time"
)

// Meeting represents a meeting entity in the database
type Meeting struct {
	ID            int64          `json:"id"`
	Name          string         `json:"name"` // Unique name, default to uploaded filename
	Transcript    sql.NullString `json:"transcript,omitempty"`
	Summary       sql.NullString `json:"summary,omitempty"`
	ChatHistory   sql.NullString `json:"chat_history,omitempty"` // Store as JSON string
	Remark        sql.NullString `json:"remark,omitempty"`
	AudioFilename string         `json:"audio_filename"` // Original uploaded audio/text filename
	UploadedAt    time.Time      `json:"uploaded_at"`
	ModifiedAt    time.Time      `json:"modified_at"`
	DeletedAt     sql.NullTime   `json:"-"` // Use '-' to exclude from default JSON responses
}


// MeetingRepository defines the interface for meeting data operations
type MeetingRepository interface {
	CreateMeeting(meeting *Meeting) (int64, error)
	ListMeetings() ([]Meeting, error)
	GetMeetingByID(id int64) (*Meeting, error)
	UpdateMeeting(id int64, meeting *Meeting) error
}

// --- Existing structs (keeping them for now, might need adjustment later) ---

// PostMeetingResponse represents the response for creating a meeting
// Let's update this to return the ID as int64
type PostMeetingResponse struct {
	ID int64 `json:"id"`
}

// GetMeetingsResponse represents the response for listing meetings
// This might need adjustment based on how we list meetings (e.g., excluding deleted ones)
type GetMeetingsResponse struct {
	Meetings []Meeting `json:"meetings"`
}

// ChatMessage represents a chat message in the SSE stream
type ChatMessage struct {
	Data string `json:"data"`
}


// SummaryResponse represents the structured JSON response from the LLM
type SummaryResponse struct {
	Summary string   `json:"summary"`
	Tasks   []string `json:"tasks"`
}
