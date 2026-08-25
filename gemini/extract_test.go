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

		got, err := extractText(resp)
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

		got, err := extractText(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "答えは42です" {
			t.Errorf("思考サマリが本文に混入しています: got %q", got)
		}
	})

	t.Run("nil パートを飛ばすこと", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonStop, nil, &genai.Part{Text: "ok"})

		got, err := extractText(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "ok" {
			t.Errorf("got %q, want \"ok\"", got)
		}
	})

	t.Run("空レスポンスは ErrEmptyResponse になること", func(t *testing.T) {
		_, err := extractText(&genai.GenerateContentResponse{})

		if !errors.Is(err, ErrEmptyResponse) {
			t.Errorf("ErrEmptyResponse を期待しましたが: %v", err)
		}
		if errors.Is(err, ErrBlocked) {
			t.Error("空レスポンスが ErrBlocked に分類されています")
		}
	})

	t.Run("nil の候補スロットは ErrEmptyResponse になること", func(t *testing.T) {
		// 候補スロット自体もサーバー由来の値で nil があり得る。以前は
		// Candidates[0] を無防備に触っていたため panic する経路だった。
		_, err := extractText(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{nil}})

		if !errors.Is(err, ErrEmptyResponse) {
			t.Errorf("ErrEmptyResponse を期待しましたが: %v", err)
		}
	})

	t.Run("ブロックは ErrBlocked と FinishReason を持つこと", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonSafety, &genai.Part{Text: "..."})

		_, err := extractText(resp)

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
	t.Run("サンプリングパラメータが SDK に渡ること", func(t *testing.T) {
		cfg, err := buildGenerateConfig(GenerateOptions{
			Temperature:     Ptr[float32](0),
			TopP:            Ptr[float32](0.9),
			TopK:            Ptr[float32](40),
			MaxOutputTokens: 2048,
			StopSequences:   []string{"END"},
		}, false)
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

	// 思考設定そのものの網羅は TestBuildThinkingConfig にあります。ここは
	// buildGenerateConfig がその結果を素通しするかだけを見ます。
	t.Run("ThinkingBudget 0 で思考を無効化できること", func(t *testing.T) {
		cfg, err := buildGenerateConfig(GenerateOptions{ThinkingBudget: Ptr[int32](0)}, false)
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
}

// TestExtractText_UnsetFinishReason は、終了理由が未設定のレスポンスを
// ブロック扱いしないことを検証します。
//
// genai.FinishReason のゼロ値は "" で、SDK 定数 FinishReasonUnspecified
// ("FINISH_REASON_UNSPECIFIED") とは別値です。サーバーは終了理由を含まない
// レスポンスを返すことがあり、ここを取り違えると正常な応答が
// 「ブロックされました」で失敗します。
func TestExtractText_UnsetFinishReason(t *testing.T) {
	t.Run("ゼロ値の FinishReason はブロック扱いしない", func(t *testing.T) {
		resp := respWithParts("", &genai.Part{Text: "チャンク"})

		got, err := extractText(resp)
		if err != nil {
			t.Fatalf("終了理由なしのレスポンスがブロック判定されました: %v", err)
		}
		if got != "チャンク" {
			t.Errorf("got %q, want \"チャンク\"", got)
		}
	})

	t.Run("FinishReasonUnspecified もブロック扱いしない", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonUnspecified, &genai.Part{Text: "本文"})

		if _, err := extractText(resp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("STOP はブロック扱いしない", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonStop, &genai.Part{Text: "本文"})

		if _, err := extractText(resp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("MAX_TOKENS はブロック扱いする", func(t *testing.T) {
		resp := respWithParts(genai.FinishReasonMaxTokens, &genai.Part{Text: "途中"})

		if _, err := extractText(resp); !errors.Is(err, ErrBlocked) {
			t.Errorf("ErrBlocked を期待しましたが: %v", err)
		}
	})
}

func TestBuildThinkingConfig(t *testing.T) {
	tests := []struct {
		name      string
		opts      GenerateOptions
		wantNil   bool
		wantLevel genai.ThinkingLevel
		wantBudge *int32
	}{
		{
			name:    "未指定なら nil（モデル既定の思考挙動を上書きしない）",
			opts:    GenerateOptions{},
			wantNil: true,
		},
		{
			name:      "ThinkingBudget のみ",
			opts:      GenerateOptions{ThinkingBudget: Ptr[int32](0)},
			wantBudge: Ptr[int32](0),
		},
		{
			name:      "ThinkingLevel のみ",
			opts:      GenerateOptions{ThinkingLevel: genai.ThinkingLevelLow},
			wantLevel: genai.ThinkingLevelLow,
		},
		{
			name:      "両方指定なら ThinkingLevel を優先し Budget は送らない",
			opts:      GenerateOptions{ThinkingLevel: genai.ThinkingLevelHigh, ThinkingBudget: Ptr[int32](4096)},
			wantLevel: genai.ThinkingLevelHigh,
		},
		{
			name:      "Unspecified は未指定扱い",
			opts:      GenerateOptions{ThinkingLevel: genai.ThinkingLevelUnspecified, ThinkingBudget: Ptr[int32](128)},
			wantBudge: Ptr[int32](128),
		},
		{
			name: "IncludeThoughts だけでも設定を送る",
			opts: GenerateOptions{IncludeThoughts: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildThinkingConfig(tt.opts)

			if tt.wantNil {
				if got != nil {
					t.Fatalf("nil を期待しましたが: %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("ThinkingConfig が nil です")
			}
			if got.ThinkingLevel != tt.wantLevel {
				t.Errorf("ThinkingLevel = %q, want %q", got.ThinkingLevel, tt.wantLevel)
			}
			switch {
			case tt.wantBudge == nil && got.ThinkingBudget != nil:
				t.Errorf("ThinkingBudget = %v, want nil", *got.ThinkingBudget)
			case tt.wantBudge != nil && got.ThinkingBudget == nil:
				t.Errorf("ThinkingBudget = nil, want %v", *tt.wantBudge)
			case tt.wantBudge != nil && *got.ThinkingBudget != *tt.wantBudge:
				t.Errorf("ThinkingBudget = %v, want %v", *got.ThinkingBudget, *tt.wantBudge)
			}
			if got.IncludeThoughts != tt.opts.IncludeThoughts {
				t.Errorf("IncludeThoughts = %v, want %v", got.IncludeThoughts, tt.opts.IncludeThoughts)
			}
		})
	}
}

func TestBuildGenerateConfig_ResponseSchema(t *testing.T) {
	schema := &genai.Schema{Type: genai.TypeObject}
	jsonSchema := map[string]any{"type": "object"}

	t.Run("ResponseSchema のみ", func(t *testing.T) {
		cfg, err := buildGenerateConfig(GenerateOptions{ResponseSchema: schema}, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ResponseSchema == nil || cfg.ResponseJsonSchema != nil {
			t.Errorf("ResponseSchema だけが送られるべきです: schema=%v json=%v",
				cfg.ResponseSchema, cfg.ResponseJsonSchema)
		}
	})

	t.Run("両方指定なら ResponseJSONSchema を優先すること", func(t *testing.T) {
		cfg, err := buildGenerateConfig(GenerateOptions{
			ResponseSchema:     schema,
			ResponseJSONSchema: jsonSchema,
		}, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ResponseJsonSchema == nil {
			t.Error("ResponseJsonSchema が送られていません")
		}
		if cfg.ResponseSchema != nil {
			t.Error("両方送るとどちらが効くか不定になるため ResponseSchema は送らないべきです")
		}
	})
}

// TestResponseFromGenAISkipsNilParts verifies a nil part in the response does not panic.
// Parts come from the server, so no amount of input validation rules them out — extractText
// already skips them and the inline-data path has to do the same.
func TestResponseFromGenAISkipsNilParts(t *testing.T) {
	resp := respWithParts(genai.FinishReasonStop,
		nil,
		&genai.Part{Text: "解説"},
		&genai.Part{InlineData: &genai.Blob{MIMEType: "image/png", Data: []byte("img")}},
		nil,
		&genai.Part{InlineData: &genai.Blob{MIMEType: "audio/mpeg", Data: []byte("snd")}},
	)

	got, err := responseFromGenAI(resp)
	if err != nil {
		t.Fatalf("responseFromGenAI() error = %v", err)
	}
	if got.Text != "解説" {
		t.Errorf("Text = %q, want %q", got.Text, "解説")
	}
	if len(got.Attachments) != 2 {
		t.Fatalf("Attachments = %d, want 2", len(got.Attachments))
	}
	if len(got.Images) != 1 || string(got.Images[0]) != "img" {
		t.Errorf("Images = %q, want the png bytes", got.Images)
	}
	if len(got.Audios) != 1 || string(got.Audios[0]) != "snd" {
		t.Errorf("Audios = %q, want the mpeg bytes", got.Audios)
	}
}
