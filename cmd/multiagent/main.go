package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"meetingagent/config"
	"os"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/multiagent/host"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// MockMeeting is a mock meeting for chat context
const mockMeeting = `
会议日期：2025年4月20日
与会人员：张三、李四、王五
主题：进度跟进会议

讨论要点：
1. 张三报告了产品A的开发进度，目前完成了80%的开发工作
2. 李四提出了UI设计的修改建议，需要在周三之前完成
3. 王五分享了测试结果，发现了3个关键bug，需要在下周一修复
4. 团队讨论了市场需求变化，决定增加一个新功能

任务分配：
- 张三：修复主界面崩溃问题，周五前完成
- 李四：优化登录界面UI，周三前完成
- 王五：完成自动化测试脚本，下周二前完成
- 所有人：准备下周的版本发布会议
`

// TaskAction represents a task action intent
type TaskAction struct {
	MeetingID string `json:"meeting_id"`
	TaskIndex string `json:"task_index"`
	Status    string `json:"status"`
}

type logCallback struct{}

func (l *logCallback) OnHandOff(ctx context.Context, info *host.HandOffInfo) context.Context {
	println("\nHandOff to", info.ToAgentName, "with argument", info.Argument)
	return ctx
}

func main() {
	ctx := context.Background()

	// Initialize the host and specialist agents
	h, err := Host(ctx)
	if err != nil {
		log.Fatalf("Failed to create host agent: %v", err)
	}

	// Create task management specialist
	taskSpecialist, err := newTaskManagementSpecialist(ctx)
	if err != nil {
		log.Fatalf("Failed to create task management specialist: %v", err)
	}

	// Create chat specialist
	chatSpecialist, err := newChatSpecialist(ctx)
	if err != nil {
		log.Fatalf("Failed to create chat specialist: %v", err)
	}

	// Register specialists with the host
	hostMA, err := host.NewMultiAgent(ctx, &host.MultiAgentConfig{
		Host: *h,
		Specialists: []*host.Specialist{
			taskSpecialist,
			chatSpecialist,
		},
	 })
	if err != nil {
		log.Fatalf("Failed to create multi-agent: %v", err)
	}

	fmt.Println("会议助手已启动! 输入'exit'退出.")
	fmt.Println("示例指令:")
	fmt.Println("- 完成会议1的第2个任务")
	fmt.Println("- 把会议3的第1个任务标记为未完成")
	fmt.Println("- 告诉我会议中讨论了什么?")
	fmt.Println("- 会议有哪些任务分配?")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n请输入指令: ")
		var input string
		if scanner.Scan() {
			input = scanner.Text()
			if input == "exit" {
				break
			}
		}

		msg := &schema.Message{
			Role:    schema.User,
			Content: input,
		}

		// Setup callbacks to receive streaming output
		cb := &logCallback{}

		// Process with multi-agent
		out, err := hostMA.Stream(ctx, []*schema.Message{msg}, host.WithAgentCallbacks(cb))
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		defer out.Close()

		fmt.Println("\n回答:")
		for {
			msg, err := out.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				fmt.Printf("Error receiving: %v\n", err)
				break
			}
			fmt.Print(msg.Content)
		}
	}
}

// newHost creates and configures the host agent that performs intent detection
func Host(ctx context.Context) (*host.Host, error) {

	// Load application configuration
	conf, err := config.LoadConfig("config.yml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}


	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  conf.APIKey,
		BaseURL: conf.BaseURL,
		Model:   conf.Summary.Model, // Using the same model for intent detection
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chat model: %v", err)
	}

	return &host.Host{
		ChatModel: cm,
		SystemPrompt: `你是一个会议助手，负责理解用户意图并将请求路由到合适的专家处理。
你需要判断用户的指令是属于以下哪种类型：
1. 任务管理意图：例如"完成会议1的第2个任务"、"把任务3标记为已完成"等，这些请求应该由"task_management"专家处理
2. 聊天意图：例如询问会议内容、任务分配、讨论要点等，这些请求应该由"meeting_chat"专家处理

你只需要识别意图并正确路由，不需要自己处理请求。`,
	}, nil
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
			userMessage := input[len(input)-1].Content

			// Use LLM to extract task parameters
			extractionMsg := []*schema.Message{
				{
					Role: schema.System,
					Content: `从用户消息中提取任务管理参数。提取会议ID、任务索引和目标状态。
返回一个严格的JSON格式，不要包含任何额外的文本：
{
  "meeting_id": "会议ID编号",
  "task_index": "任务序号",
  "status": "true表示完成，false表示未完成"
}`,
				},
				{
					Role:    schema.User,
					Content: userMessage,
				},
			}

			response, err := cm.Generate(ctx, extractionMsg)
			if err != nil {
				return nil, fmt.Errorf("failed to extract task parameters: %v", err)
			}

			// Parse the JSON response
			var taskAction TaskAction
			err = json.Unmarshal([]byte(response.Content), &taskAction)
			if err != nil {
				return nil, fmt.Errorf("failed to parse task parameters: %v", err)
			}

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

			var taskActionStatusStr string
			if taskAction.Status == "true" {
				taskActionStatusStr = "完成"
			} else {
				taskActionStatusStr = "将"
			}

			return &schema.Message{
				Role: schema.Assistant,
				Content: fmt.Sprintf("✓ 成功%s会议%s的第%s个任务\n\n%s",
					taskActionStatusStr,
					taskAction.MeetingID,
					taskAction.TaskIndex,
					result.Content),
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
			// In a real implementation, we'd fetch the meeting content from the database
			// Here we'll use mock data for simplicity
			meetingContent := mockMeeting

			// Create messages for the chat model
			messages := []*schema.Message{
				{
					Role: schema.System,
					Content: `你是一个会议助手，负责回答有关会议内容的问题。
基于提供的会议记录回答用户的问题。如果问题不相关，温和地引导用户回到会议主题。
只使用提供的会议记录中的信息，不要编造不存在的内容。`,
				},
				{
					Role:    schema.User,
					Content: fmt.Sprintf("会议记录:\n%s", meetingContent),
				},
			}

			// Add user messages
			if len(input) > 0 {
				messages = append(messages, &schema.Message{
					Role:    schema.User,
					Content: input[len(input)-1].Content,
				})
			}

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
