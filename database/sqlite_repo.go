package database

import (
	"database/sql"
	"fmt"
	"meetingagent/models"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SQLiteRepository implements the models.MeetingRepository interface for SQLite.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new instance of SQLiteRepository.
func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

// CreateMeeting inserts a new meeting record into the database.
func (r *SQLiteRepository) CreateMeeting(meeting *models.Meeting) (int64, error) {
	query := `
INSERT INTO meetings (
    name, transcript, summary, chat_history, remark,
    audio_filename, uploaded_at, modified_at, deleted_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
`
	// Ensure timestamps are set if not already
	if meeting.UploadedAt.IsZero() {
		meeting.UploadedAt = time.Now()
	}
	if meeting.ModifiedAt.IsZero() {
		meeting.ModifiedAt = time.Now()
	}

	result, err := r.db.Exec(query,
		meeting.Name,
		meeting.Transcript,
		meeting.Summary,
		meeting.ChatHistory,
		meeting.Remark,
		meeting.AudioFilename,
		meeting.UploadedAt,
		meeting.ModifiedAt,
		meeting.DeletedAt,
	)
	if err != nil {
		// TODO: Check for specific errors like UNIQUE constraint violation (sqlite3.ErrConstraintUnique)
		return 0, fmt.Errorf("failed to insert meeting: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return id, nil
}

// ListMeetings retrieves all meetings that haven't been deleted
func (r *SQLiteRepository) ListMeetings() ([]models.Meeting, error) {
	query := `
SELECT id, name, transcript, summary, chat_history, remark,
       audio_filename, uploaded_at, modified_at, deleted_at
FROM meetings
WHERE deleted_at IS NULL
ORDER BY uploaded_at DESC;
`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query meetings: %w", err)
	}
	defer rows.Close()

	var meetings []models.Meeting
	for rows.Next() {
		var m models.Meeting
		err := rows.Scan(
			&m.ID,
			&m.Name,
			&m.Transcript,
			&m.Summary,
			&m.ChatHistory,
			&m.Remark,
			&m.AudioFilename,
			&m.UploadedAt,
			&m.ModifiedAt,
			&m.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan meeting row: %w", err)
		}
		meetings = append(meetings, m)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating meeting rows: %w", err)
	}

	return meetings, nil
}

// InitSchema creates the necessary tables if they don't exist.
func InitSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS meetings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    transcript TEXT,
    summary TEXT,
    chat_history TEXT,
    remark TEXT,
    audio_filename TEXT NOT NULL,
    uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    modified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL
);

CREATE INDEX IF NOT EXISTS idx_meetings_name ON meetings (name);
`
	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}
	fmt.Println("Database schema initialized successfully.")
	return nil
}
