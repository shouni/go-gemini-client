package gemini

import (
	"errors"
	"testing"

	"google.golang.org/genai"
)

func respWithParts(reason genai.FinishReason, parts ...*genai.Part) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{FinishReason: reason, Content: &genai.Content{Parts: parts}},
		},
	}
}

func TestExtractText(t *testing.T) {
	t.Run("複数のテキストパートが連結されること", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonStop,
			&genai.Part{Text: "こんにちは"},
			&genai.Part{Text: "世界"},
		)

		got, err := extractText(resp, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "こんにちは世界" {
			t.Errorf("テキストが欠落しています: got %q", got)
		}
	})

	t.Run("思考パートが本文に混入しないこと", func(t *testing.T) {
		// 思考機能が有効なモデルは思考サマリを本文より前に返す。
		resp := respWithParts(genai.FinishReasonStop,
			&genai.Part{Text: "まず前提を整理する…", Thought: true},
			&genai.Part{Text: "答えは42です"},
		)

		got, err := extractText(resp, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "答えは42です" {
			t.Errorf("思考サマリが本文に混入しています: got %q", got)
		}
	})

	t.Run("nil パートを飛ばすこと", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonStop, nil, &genai.Part{Text: "ok"})

		got, err := extractText(resp, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "ok" {
			t.Errorf("got %q, want \"ok\"", got)
		}
	})

	t.Run("空レスポンスは ErrEmptyResponse になること", func(t *testing.T) {
		_, err := extractText(&genai.GenerateContentResponse{}, false)

		if !errors.Is(err, ErrEmptyResponse) {
			t.Errorf("ErrEmptyResponse を期待しましたが: %v", err)
		}
		if errors.Is(err, ErrBlocked) {
			t.Error("空レスポンスが ErrBlocked に分類されています")
		}
	})

	t.Run("lenient では空レスポンスを許容すること", func(t *testing.T) {
		got, err := extractText(&genai.GenerateContentResponse{}, true)
		if err != nil || got != "" {
			t.Errorf("got (%q, %v), want (\"\", nil)", got, err)
		}
	})

	t.Run("ブロックは ErrBlocked と FinishReason を持つこと", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonSafety, &genai.Part{Text: "..."})

		_, err := extractText(resp, false)

		if !errors.Is(err, ErrBlocked) {
			t.Fatalf("ErrBlocked を期待しましたが: %v", err)
		}
		apiErr, ok := errors.AsType[*APIResponseError](err)
		if !ok {
			t.Fatalf("*APIResponseError を期待しましたが: %T", err)
		}
		if apiErr.FinishReason != genai.FinishReasonSafety {
			t.Errorf("FinishReason = %v, want %v", apiErr.FinishReason, genai.FinishReasonSafety)
		}
	})
}

func TestExtractThoughts(t *testing.T) {
	t.Run("思考パートのみを連結すること", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonStop,
			&genai.Part{Text: "思考A", Thought: true},
			&genai.Part{Text: "本文"},
			&genai.Part{Text: "思考B", Thought: true},
		)

		if got := extractThoughts(resp); got != "思考A思考B" {
			t.Errorf("got %q, want \"思考A思考B\"", got)
		}
	})

	t.Run("思考がなければ空文字列", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonStop, &genai.Part{Text: "本文"})

		if got := extractThoughts(resp); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("nil レスポンスでも panic しないこと", func(t *testing.T) {
		if got := extractThoughts(nil); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestBuildGenerateConfig_SamplingParams(t *testing.T) {
	c := &Client{}

	t.Run("サンプリングパラメータが SDK に渡ること", func(t *testing.T) {
		cfg, err := c.buildGenerateConfig(GenerateOptions{
			Temperature:     Ptr[float32](0),
			TopP:            Ptr[float32](0.9),
			TopK:            Ptr[float32](40),
			MaxOutputTokens: 2048,
			StopSequences:   []string{"END"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Temperature 0 は「未設定」ではなく「決定的な出力」を意味するため、
		// ポインタで区別できていることを確認する。
		if cfg.Temperature == nil || *cfg.Temperature != 0 {
			t.Errorf("Temperature = %v, want 0", cfg.Temperature)
		}
		if cfg.TopP == nil || *cfg.TopP != 0.9 {
			t.Errorf("TopP = %v, want 0.9", cfg.TopP)
		}
		if cfg.TopK == nil || *cfg.TopK != 40 {
			t.Errorf("TopK = %v, want 40", cfg.TopK)
		}
		if cfg.MaxOutputTokens != 2048 {
			t.Errorf("MaxOutputTokens = %d, want 2048", cfg.MaxOutputTokens)
		}
		if len(cfg.StopSequences) != 1 || cfg.StopSequences[0] != "END" {
			t.Errorf("StopSequences = %v", cfg.StopSequences)
		}
	})

	t.Run("未指定なら ThinkingConfig を送らないこと", func(t *testing.T) {
		cfg, err := c.buildGenerateConfig(GenerateOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ThinkingConfig != nil {
			t.Error("未指定時に ThinkingConfig を送るとモデル既定の思考挙動を上書きしてしまいます")
		}
	})

	t.Run("ThinkingBudget 0 で思考を無効化できること", func(t *testing.T) {
		cfg, err := c.buildGenerateConfig(GenerateOptions{ThinkingBudget: Ptr[int32](0)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ThinkingConfig == nil {
			t.Fatal("ThinkingConfig が設定されていません")
		}
		if cfg.ThinkingConfig.ThinkingBudget == nil || *cfg.ThinkingConfig.ThinkingBudget != 0 {
			t.Errorf("ThinkingBudget = %v, want 0", cfg.ThinkingConfig.ThinkingBudget)
		}
	})

	t.Run("IncludeThoughts のみでも ThinkingConfig を送ること", func(t *testing.T) {
		cfg, err := c.buildGenerateConfig(GenerateOptions{IncludeThoughts: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ThinkingConfig == nil || !cfg.ThinkingConfig.IncludeThoughts {
			t.Error("IncludeThoughts が反映されていません")
		}
	})
}

// TestExtractText_UnsetFinishReason は、終了理由が未設定のレスポンスを
// ブロック扱いしないことを検証します。
//
// genai.FinishReason のゼロ値は "" で、SDK 定数 FinishReasonUnspecified
// ("FINISH_REASON_UNSPECIFIED") とは別値です。ストリーミングの中間チャンクは
// 終了理由を含まないため、ここを取り違えると通常のストリーミングが
// 「ブロックされました」で失敗します。
func TestExtractText_UnsetFinishReason(t *testing.T) {
	t.Run("ゼロ値の FinishReason はブロック扱いしない", func(t *testing.T) {
		resp := respWithParts("", &genai.Part{Text: "チャンク"})

		got, err := extractText(resp, true)
		if err != nil {
			t.Fatalf("ストリーミング中間チャンクがブロック判定されました: %v", err)
		}
		if got != "チャンク" {
			t.Errorf("got %q, want \"チャンク\"", got)
		}
	})

	t.Run("FinishReasonUnspecified もブロック扱いしない", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonUnspecified, &genai.Part{Text: "本文"})

		if _, err := extractText(resp, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("STOP はブロック扱いしない", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonStop, &genai.Part{Text: "本文"})

		if _, err := extractText(resp, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("MAX_TOKENS はブロック扱いする", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonMaxTokens, &genai.Part{Text: "途中"})

		if _, err := extractText(resp, false); !errors.Is(err, ErrBlocked) {
			t.Errorf("ErrBlocked を期待しましたが: %v", err)
		}
	})
}
