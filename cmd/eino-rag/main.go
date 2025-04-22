package main

import (
	"context"
	"fmt"
	"github.com/cloudwego/eino/components/document"
	"github.com/google/uuid"
	"io"
)

const (
	prefix = "MeetingTranscripts:"
	index  = "OuterIndex"
)

func main() {
	ctx := context.Background()

	r, err := InitRAGEngine(ctx, index, prefix)
	if err != nil {
		panic(err)
	}

	doc, err := r.Loader.Load(ctx, document.Source{
		URI: "./example/content.txt",
	})
	if err != nil {
		panic(err)
	}

	docs, err := r.Splitter.Transform(ctx, doc)
	if err != nil {
		panic(err)
	}

	for _, d := range docs {
		myUUid, _ := uuid.NewUUID()
		d.ID = myUUid.String()
	}
	fmt.Println(r)

	err = r.InitVectorIndex(ctx)
	if err != nil {
		panic(err)
	}

	_, err = r.Indexer.Store(ctx, docs)
	if err != nil {
		panic(err)
	}

	var query string

	for {
		_, _ = fmt.Scan(&query)
		output, err := r.Generate(ctx, query)
		if err != nil {
			panic(err)
		}
		for {
			o, err := output.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				panic(err)
			}
			fmt.Println(o.Content)
		}
	}
}
