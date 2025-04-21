package main

import (
	"context"
	"database/sql"
	"log"
	"path/filepath"
	"time"

	"meetingagent/config"
	"meetingagent/database" // Import the database package
	"meetingagent/handlers"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	_ "github.com/mattn/go-sqlite3" // Import SQLite driver for side effects
)

const dbFile = "meetings.db"    // Define database file name
const configFile = "config.yml" // Define config file name

func main() {
	// --- Configuration Setup ---
	configPath := filepath.Join(".", configFile)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Loaded configuration: API Key=%s, Model=%s", cfg.APIKey[:8]+"...", cfg.Summary.Model)

	// --- Database Setup ---
	db, err := sql.Open("sqlite3", dbFile+"?_foreign_keys=on") // Enable foreign keys if needed later
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Ping the database to ensure connection is valid
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Initialize database schema (create tables if they don't exist)
	if err := database.InitSchema(db); err != nil {
		log.Fatalf("Failed to initialize database schema: %v", err)
	}

	// Create repository instance
	repo := database.NewSQLiteRepository(db)

	// Inject repository into handlers
	handlers.SetMeetingRepository(repo)
	// --- End Database Setup ---

	h := server.Default()
	h.Use(Logger())

	// Register API routes first
	h.POST("/meeting", handlers.CreateMeeting)
	h.GET("/meeting", handlers.ListMeetings)
	h.GET("/summary", handlers.GetMeetingSummary)
	h.GET("/chat", handlers.HandleChat)

	// Serve static files
	h.StaticFS("/", &app.FS{
		Root:               "./static",
		PathRewrite:        app.NewPathSlashesStripper(1),
		IndexNames:         []string{"index.html"},
		GenerateIndexPages: true,
	})

	// Start server
	h.Spin()
}

func Logger() app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		start := time.Now()
		path := string(ctx.Request.URI().Path())
		query := string(ctx.Request.URI().QueryString())
		if query != "" {
			path = path + "?" + query
		}

		// Process request
		ctx.Next(c)

		// Calculate latency
		latency := time.Since(start)

		// Get response status code
		statusCode := ctx.Response.StatusCode()

		// Log request details
		hlog.CtxInfof(c, "[HTTP] %s %s - %d - %v",
			ctx.Request.Method(),
			path,
			statusCode,
			latency,
		)
	}
}
