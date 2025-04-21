package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	// einomcp "github.com/cloudwego/eino-ext/components/tool/mcp"
)

func main() {
	// // exec shell path
	// cmd := exec.Command("ls", "./cmd/tasktool-mcpserver/server.go")
	// output, err := cmd.Output()
	// if err != nil {
	// 	fmt.Println("执行命令出错:", err)
	// 	return
	// }

	// // 打印命令的输出结果
	// fmt.Println(string(output))

	// stdio client
	cli, err := client.NewStdioMCPClient("go", []string{}, "run", "./cmd/tasktool-mcpserver/server.go")
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "example-client",
		Version: "1.0.0",
	}
	_, err = cli.Initialize(ctx, initRequest)

	// tools, err := einomcp.GetTools(ctx, &einomcp.Config{Cli: cli})
	// if err != nil {
	// 	panic(err)
	// }

	// for i, mcpTool := range tools {
	// 	fmt.Println(i, ":")
	// 	info, err := mcpTool.Info(ctx)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	fmt.Println("Name:", info.Name)
	// 	fmt.Println("Desc:", info.Desc)
	// 	fmt.Println()
	// }

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
				"meeting_id": "1",
				"task_index": "2",
				"status":     "true",
			},
		},
	}
	result, err := cli.CallTool(ctx, request)
	if err != nil {
		log.Panic(err)
	}
	fmt.Println("Result:", result.Content)

}
