package gemini_test

import (
	"errors"
	"fmt"

	"github.com/shouni/go-gemini-client/gemini"
)

// 生成パラメータはリクエストごとに GenerateOptions で指定します。
// Temperature のようにゼロ値が意味を持つ項目はポインタなので、Ptr を使います。
func ExampleGenerateOptions() {
	opts := gemini.GenerateOptions{
		SystemPrompt:    "あなたは簡潔に答えるアシスタントです。",
		Temperature:     new(float32(0)), // 0 = 最も決定的
		MaxOutputTokens: 1024,
		StopSequences:   []string{"###"},
	}

	fmt.Println(*opts.Temperature, opts.MaxOutputTokens)
	// Output: 0 1024
}

// 思考機能 (Gemini 2.5 以降) はコストとレイテンシに直結するため、
// ThinkingBudget を 0 にして明示的に無効化できます。
func ExampleGenerateOptions_thinking() {
	// 思考を無効化してレイテンシを抑える
	fast := gemini.GenerateOptions{ThinkingBudget: new(int32(0))}

	// 思考を有効にしつつサマリも受け取る（Response.Thoughts に入る）
	verbose := gemini.GenerateOptions{
		ThinkingBudget:  new(int32(2048)),
		IncludeThoughts: true,
	}

	fmt.Println(*fast.ThinkingBudget, *verbose.ThinkingBudget, verbose.IncludeThoughts)
	// Output: 0 2048 true
}

// 生成失敗の理由は errors.Is / errors.AsType で分類できます。
// ブロックはリトライしても解決しないため、プロンプトの見直しが必要です。
func ExampleAPIResponseError() {
	// 実際には client.GenerateContent などが返すエラーを受け取ります。
	var err error = &gemini.APIResponseError{
		Reason:  gemini.ErrEmptyResponse,
		Message: "Gemini APIから空のレスポンスが返されました",
	}

	switch {
	case errors.Is(err, gemini.ErrBlocked):
		fmt.Println("blocked: プロンプトを見直してください")
	case errors.Is(err, gemini.ErrEmptyResponse):
		fmt.Println("empty: 候補が返りませんでした")
	}

	if apiErr, ok := errors.AsType[*gemini.APIResponseError](err); ok {
		fmt.Println("reason:", apiErr.Reason)
	}

	// Output:
	// empty: 候補が返りませんでした
	// reason: gemini: empty response
}

// CleanJSONResponse は構造化出力に混じる Markdown 装飾や末尾ノイズを取り除きます。
func ExampleCleanJSONResponse() {
	fmt.Println(gemini.CleanJSONResponse("```json\n{\"title\":\"ok\"}\n```"))
	fmt.Println(gemini.CleanJSONResponse(`{"title":"ok"} このあとに説明が続く`))
	fmt.Println(gemini.CleanJSONResponse(`前置き [1,2,3] 後置き`))

	// Output:
	// {"title":"ok"}
	// {"title":"ok"}
	// [1,2,3]
}

// Config は Vertex AI と Gemini API を排他的に指定します。
// HTTPClient を差し替えると、SSRF 対策済みクライアントなどを注入できます。
func ExampleConfig() {
	vertex := gemini.Config{
		ProjectID:  "my-project",
		LocationID: "us-central1",
	}
	studio := gemini.Config{
		APIKey: "AIza...",
	}

	fmt.Println(vertex.ProjectID != "" && vertex.APIKey == "")
	fmt.Println(studio.APIKey != "" && studio.ProjectID == "")
	// Output:
	// true
	// true
}
