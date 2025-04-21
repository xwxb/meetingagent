package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"meetingagent/config"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/multiagent/host"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func Init() {
	bgCtx := context.Background()
	if err := initSummaryChatModel(bgCtx); err != nil {
		log.Fatalf("failed to init SummaryChatModel: %v", err)
	}
	taskManager, err := newTaskManagementSpecialist(bgCtx)
	if err != nil {
		log.Fatalf("failed to create task management specialist: %v", err)
	}
	chatManager, err := newChatSpecialist(bgCtx)
	if err != nil {
		log.Fatalf("failed to create chat specialist: %v", err)
	}
	chatAgent, err := newChatAgent(bgCtx)
	if err != nil {
		log.Fatalf("failed to create chat agent: %v", err)
	}
	if err := initHostMA(bgCtx, chatAgent, []*host.Specialist{taskManager, chatManager}); err != nil {
		log.Fatalf("failed to init host multi-agent: %v", err)
	}

	log.Printf("✔ ChatModels and Agents initialized")
}

var SummaryChatModel *ark.ChatModel
var HostMA *host.MultiAgent

func initSummaryChatModel(ctx context.Context) error {
	if config.AppConfig == nil {
		return fmt.Errorf("application config not initialized")
	}
	if SummaryChatModel != nil {
		return nil // Already initialized
	}

	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  config.AppConfig.APIKey,
		BaseURL: config.AppConfig.BaseURL,
		Model:   config.AppConfig.Summary.Model,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize summary chat model: %v", err)
	}

	SummaryChatModel = cm
	return nil
}

// handleTaskWithMCP handles the MCP connection and task status update
func handleTaskWithMCP(ctx context.Context, taskAction TaskAction) ([]mcp.Content, error) {
	// Connect to MCP server
	cli, err := client.NewStdioMCPClient("go", []string{}, "run", "./cmd/tasktool-mcpserver/server.go")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MCP server: %v", err)
	}

	// Initialize MCP connection
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "meeting-task-client",
		Version: "1.0.0",
	}
	_, err = cli.Initialize(ctx, initRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP connection: %v", err)
	}

	// Call the update_task_status tool
	request := mcp.CallToolRequest{
		Params: struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments,omitempty"`
			Meta      *struct {
				ProgressToken mcp.ProgressToken `json:"progressToken,omitempty"`
			} `json:"_meta,omitempty"`
		}{
			Name: "update_task_status",
			Arguments: map[string]interface{}{
				"meeting_id": taskAction.MeetingID,
				"task_index": taskAction.TaskIndex,
				"status":     taskAction.Status,
			},
		},
	}
	fmt.Println("Calling MCP tool with request:", request)

	result, err := cli.CallTool(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to call MCP tool: %v", err)
	}

	return result.Content, nil
}

// newTaskManagementSpecialist creates a specialist that handles task management via MCP
func newTaskManagementSpecialist(ctx context.Context) (*host.Specialist, error) {
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  config.AppConfig.APIKey,
		BaseURL: config.AppConfig.BaseURL,
		Model:   config.AppConfig.Summary.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chat model for task specialist: %v", err)
	}

	return &host.Specialist{
		AgentMeta: host.AgentMeta{
			Name:        "task_management",
			IntendedUse: "处理任务状态修改请求，如标记任务完成或未完成",
		},
		Invokable: func(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (*schema.Message, error) {
			// Extract task parameters from user message
			// this call only need lastest message


			// Use LLM to extract task parameters
			extractionMsg := &schema.Message{
				Role:    schema.System,
				Content: config.AppConfig.ChatAgent.ChatSpecialist.TaskManagement.SystemMessage,
			}

			msgs := []*schema.Message{
				extractionMsg,
			}
			msgs = append(msgs, input...)  // 目前历史消息没实现，所以还是要带上所有上下文

			response, err := cm.Generate(ctx, msgs)
			if err != nil {
				return nil, fmt.Errorf("failed to extract task parameters: %v", err)
			}
			log.Default().Printf("Extracted task parameters: %s", response.Content)

			// Parse the JSON response
			var taskAction TaskAction
			err = json.Unmarshal([]byte(response.Content), &taskAction)
			if err != nil {
				return nil, fmt.Errorf("failed to parse task parameters: %v", err)
			}

			// Handle MCP logic in a separate function
			resultContents, err := handleTaskWithMCP(ctx, taskAction)
			if err != nil {
				return nil, err
			}

			var taskActionStatusStr string
			if taskAction.Status == "true" {
				taskActionStatusStr = "完成"
			} else {
				taskActionStatusStr = "将"
			}

			return &schema.Message{
				Role: schema.Assistant,
				Content: fmt.Sprintf("✓ 成功%s会议%s的第%s个任务\n\n%v",
					taskActionStatusStr,
					taskAction.MeetingID,
					taskAction.TaskIndex,
					resultContents),
			}, nil
		},
	}, nil
}

// newChatSpecialist creates a specialist that handles chat about meeting content
func newChatSpecialist(ctx context.Context) (*host.Specialist, error) {
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  config.AppConfig.APIKey,
		BaseURL: config.AppConfig.BaseURL,
		Model:   config.AppConfig.Summary.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chat model for chat specialist: %v", err)
	}

	return &host.Specialist{
		AgentMeta: host.AgentMeta{
			Name:        "meeting_chat",
			IntendedUse: "基于会议内容回答用户问题，例如讨论要点、任务分配等",
		},
		Streamable: func(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (*schema.StreamReader[*schema.Message], error) {

			// Create messages for the chat model
			messages := []*schema.Message{
				{
					Role:    schema.System,
					Content: config.AppConfig.ChatAgent.ChatSpecialist.MeetingChat.SystemMessage,
				},
			}
			// to be added user messages from handlers
			// ! to be checked
			// this call need all chat history
			messages = append(messages, input...)

			// Stream the response
			stream, err := cm.Stream(ctx, messages)
			if err != nil {
				return nil, fmt.Errorf("failed to stream chat response: %v", err)
			}

			reader, writer := schema.Pipe[*schema.Message](0)

			go func() {
				defer writer.Close()

				for {
					chunk, err := stream.Recv()
					if err != nil {
						if err != io.EOF {
							log.Printf("Error receiving chunk: %v", err)
						}
						break
					}

					writer.Send(&schema.Message{
						Role:    schema.Assistant,
						Content: chunk.Content,
					}, err)
				}
			}()

			return reader, nil
		},
	}, nil
}

// newChatAgent creates and configures the host agent that performs intent detection
func newChatAgent(ctx context.Context) (*host.Host, error) {
	conf := config.AppConfig

	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  conf.APIKey,
		BaseURL: conf.BaseURL,
		Model:   conf.Summary.Model, // Using the same model for intent detection
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chat model: %v", err)
	}

	return &host.Host{
		ChatModel:    cm,
		SystemPrompt: conf.ChatAgent.SystemMessage,
	}, nil
}

func initHostMA(ctx context.Context, h *host.Host, sps []*host.Specialist) error {
	if HostMA != nil {
		return nil // Already initialized
	}

	// Initialize the host agent with the chat model and specialists
	hostMA, err := host.NewMultiAgent(ctx, &host.MultiAgentConfig{
		Host:        *h,
		Specialists: sps,
	},
	)
	if err != nil {
		return fmt.Errorf("failed to initialize host multi-agent: %v", err)
	}
	log.Default().Printf("✔ Host multi-agent initialized with %d specialists", len(sps))

	HostMA = hostMA
	return nil
}
