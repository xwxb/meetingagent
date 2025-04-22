package main

import (
	"context"
	"fmt"
	"meetingagent/config"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"
	embedding "github.com/cloudwego/eino-ext/components/embedding/ark"
	redisInd "github.com/cloudwego/eino-ext/components/indexer/redis"
	"github.com/cloudwego/eino-ext/components/model/ark"
	redisRet "github.com/cloudwego/eino-ext/components/retriever/redis"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

func (r *RAGEngine) newChatModel(ctx context.Context) {
	m, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: r.config.APIKey,
		Model:  r.config.Summary.Model,
	})
	if err != nil {
		r.Err = err
		return
	}

	r.ChatModel = m
}

func (r *RAGEngine) newIndexer(ctx context.Context) {
	i, err := redisInd.NewIndexer(ctx, &redisInd.IndexerConfig{
		Client:           r.redis,
		KeyPrefix:        r.prefix,
		DocumentToHashes: nil,
		BatchSize:        10,
		Embedding:        r.embedder,
	})
	if err != nil {
		r.Err = err
	}
	r.Indexer = i
}

func (r *RAGEngine) InitVectorIndex(ctx context.Context) error {
	if _, err := r.redis.Do(ctx, "FT.INFO", r.indexName).Result(); err == nil {
		return nil
	}

	createIndexArgs := []interface{}{
		"FT.CREATE", r.indexName,
		"ON", "HASH",
		"PREFIX", "1", r.prefix,
		"SCHEMA",
		"content", "TEXT",
		"vector_content", "VECTOR", "FLAT",
		"6",
		"TYPE", "FLOAT32",
		"DIM", r.dimension,
		"DISTANCE_METRIC", "COSINE",
	}

	if err := r.redis.Do(ctx, createIndexArgs...).Err(); err != nil {
		return err
	}

	if _, err := r.redis.Do(ctx, "FT.INFO", r.indexName).Result(); err != nil {
		return err
	}
	return nil
}

func (r *RAGEngine) newLoader(ctx context.Context) {
	l, err := file.NewFileLoader(ctx, &file.FileLoaderConfig{
		UseNameAsID: true,
		Parser:      nil,
	})
	if err != nil {
		r.Err = err
		return
	}
	r.Loader = l
}

// reference&acknowledgement: https://github.com/OuterCyrex/Eino-example
type RAGEngine struct {
	indexName string
	prefix    string
	config    *config.Config
	dimension int

	redis    *redis.Client
	embedder *embedding.Embedder

	Err error

	Loader    *file.FileLoader
	Splitter  document.Transformer
	Retriever *redisRet.Retriever
	Indexer   *redisInd.Indexer
	ChatModel *ark.ChatModel
}

func InitRAGEngine(ctx context.Context, index string, prefix string) (*RAGEngine, error) {
	r, err := initRAGEngine(ctx, index, prefix)
	if err != nil {
		return nil, err
	}

	// try ping redis
	if err := r.redis.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	r.newLoader(ctx)
	r.newSplitter(ctx)
	r.newIndexer(ctx)
	r.newRetriever(ctx)
	r.newChatModel(ctx)

	return r, nil
}

func initRAGEngine(ctx context.Context, index string, prefix string) (*RAGEngine, error) {

	c, err := config.LoadConfig("./config.yml")
	if err != nil {
		return nil, err
	}
	// fmt.Println("embeded model: ", c.Embedder.Model)

	embedder, err := embedding.NewEmbedder(ctx, &embedding.EmbeddingConfig{
		APIKey: c.APIKey,
		Model:  c.Embedder.Model,
	})
	fmt.Println("embdder: ", embedder)

	if err != nil {
		return nil, err
	}

	return &RAGEngine{
		indexName: index,
		prefix:    prefix,
		config:    c,
		dimension: 4096,

		redis: redis.NewClient(&redis.Options{
			Addr:          c.Redis.Addr,
			Password:      c.Redis.Password,
			Protocol:      2,
			UnstableResp3: true,
		}),
		embedder: embedder,

		Loader:    nil,
		Splitter:  nil,
		Retriever: nil,
		Indexer:   nil,
		ChatModel: nil,
	}, nil
}

var systemPrompt = `
# Role: Student Learning Assistant

# Language: Chinese

- When providing assistance:
  • Be clear and concise
  • Include practical examples when relevant
  • Reference documentation when helpful
  • Suggest improvements or next steps if applicable

here's documents searched for you:
==== doc start ====
	  {documents}
==== doc end ====
`

func (r *RAGEngine) Generate(ctx context.Context, query string) (*schema.StreamReader[*schema.Message], error) {
	docs, err := r.Retriever.Retrieve(ctx, query)
	if err != nil {
		return nil, err
	}

	fmt.Println("-------------------------------------------")
	fmt.Println(docs)
	fmt.Println("-------------------------------------------")

	tpl := prompt.FromMessages(schema.FString, []schema.MessagesTemplate{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage("question: {content}"),
	}...)

	messages, err := tpl.Format(ctx, map[string]any{
		"documents": docs,
		"content":   query,
	})
	if err != nil {
		return nil, err
	}

	return r.ChatModel.Stream(ctx, messages)
}

func (r *RAGEngine) newRetriever(ctx context.Context) {
	re, err := redisRet.NewRetriever(ctx, &redisRet.RetrieverConfig{
		Client:            r.redis,
		Index:             r.indexName,
		VectorField:       "vector_content",
		DistanceThreshold: nil,
		Dialect:           2,
		ReturnFields:      []string{"vector_content", "content"},
		DocumentConverter: nil,
		TopK:              1,
		Embedding:         r.embedder,
	})

	if err != nil {
		r.Err = err
		return
	}

	r.Retriever = re
}

func (r *RAGEngine) newSplitter(ctx context.Context) {
	t, err := recursive.NewSplitter(ctx, &recursive.Config{
		ChunkSize:    1000,           // 必需：目标片段大小
		OverlapSize:  200,            // 可选：片段重叠大小
		Separators:   []string{"\n", ".", "?", "!"}, // 可选：分隔符列表
		LenFunc:      nil,            // 可选：自定义长度计算函数
		KeepType:     recursive.KeepTypeNone, // 可选：分隔符保留策略
	})
	if err != nil {
		r.Err = err
		return
	}
	r.Splitter = t
}
