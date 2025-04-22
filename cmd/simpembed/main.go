package main

import (
	"context"
	"meetingagent/config"
	"time"

	"github.com/cloudwego/eino-ext/components/embedding/ark"
)

func main() {
	ctx := context.Background()

	conf, err := config.LoadConfig("./config.yml")
	if err != nil {
		panic(err)
	}
	// fmt.Println(conf.Embedder)

	// 初始化嵌入器
	timeout := 30 * time.Second
	embedder, err := ark.NewEmbedder(ctx, &ark.EmbeddingConfig{
		APIKey:  conf.APIKey,
		Model:   conf.Embedder.Model,
		Timeout: &timeout,
	})
	if err != nil {
		panic(err)
	}

	// 生成文本向量
	texts := []string{
		"这是第一段示例文本",
		"这是第二段示例文本",
	}

	// fmt.Println("文本:", texts, "model:", conf.Embedder.Model)
	embeddings, err := embedder.EmbedStrings(ctx, texts)
	if err != nil {
		panic(err)
	}

	// 使用生成的向量
	for i, embedding := range embeddings {
		println("文本", i+1, "的向量维度:", len(embedding))
	}
}
