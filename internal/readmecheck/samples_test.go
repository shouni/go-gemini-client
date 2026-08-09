// Package readmecheck compiles the code samples from README.md so the documentation
// cannot drift from the API without CI noticing. Nothing here calls the network:
// the samples only need to type-check.
package readmecheck

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/shouni/go-gemini-client/gemini"
	"github.com/shouni/go-gemini-client/music"
	"github.com/shouni/go-gemini-client/veo"
)

// TestREADMESamplesCompile exists for its compile-time effect: if a README sample stops
// matching the API, this file stops building. It is deliberately never executed against
// a real backend.
func TestREADMESamplesCompile(t *testing.T) {
	t.Log("README samples type-check against the current API")
}

type readmeStruct struct{ Title string }

func sampleQuickstart(ctx context.Context) error {
	client, err := gemini.NewClient(ctx, gemini.Config{APIKey: "YOUR_GEMINI_API_KEY"})
	if err != nil {
		return err
	}
	resp, err := client.GenerateContent(ctx, "gemini-3.6-flash", "Goで短い俳句を書いて")
	if err != nil {
		return err
	}
	_ = resp.Text

	_, err = gemini.NewClient(ctx, gemini.Config{
		ProjectID:  "your-google-cloud-project-id",
		LocationID: "asia-northeast1",
	})
	return err
}

func sampleMultimodal(ctx context.Context, client *gemini.Client) error {
	resp, err := client.GenerateWithAttachments(ctx, "gemini-3.6-flash",
		"この画像の内容を日本語で要約してください",
		[]gemini.Attachment{
			{URI: "gs://my-bucket/sample.jpg", MIMEType: "image/jpeg"},
		},
		gemini.GenerateOptions{SystemPrompt: "簡潔に回答してください。"})
	if err != nil {
		return err
	}
	_ = resp.Text
	return nil
}

func sampleImageResponse(ctx context.Context, client *gemini.Client) error {
	seed := int64(1234)
	resp, err := client.GenerateWithAttachments(ctx, "gemini-3.1-flash-image",
		"青い招き猫のステッカー画像を生成して", nil,
		gemini.GenerateOptions{
			ResponseMIMEType: "image/png",
			AspectRatio:      "1:1",
			ImageSize:        "1K",
			Seed:             &seed,
		})
	if err != nil {
		return err
	}
	if len(resp.Images) > 0 {
		_ = resp.Images[0]
	}
	for _, attachment := range resp.Attachments {
		_ = attachment
	}
	// Response の中身の表に載せた全フィールド。
	_, _, _ = resp.Text, resp.Audios, resp.Thoughts
	_ = resp.Usage
	return nil
}

func sampleFileAPI(ctx context.Context, client *gemini.Client) error {
	f, err := os.Open("movie.mp4")
	if err != nil {
		return err
	}
	// README は慣用句の defer f.Close() を載せている。ここが _ = で受けるのは
	// このファイルの目的が型検査であり、errcheck を通す必要があるためだけ。
	defer func() { _ = f.Close() }()

	uploaded, err := client.UploadFile(ctx, f, "video/mp4", "movie.mp4")
	if err != nil {
		return err
	}
	defer func() {
		if err := client.DeleteFile(context.Background(), uploaded.Name); err != nil {
			slog.Warn("failed to delete uploaded file", "name", uploaded.Name, "error", err)
		}
	}()

	_, err = client.GenerateWithAttachments(ctx, "gemini-3.6-flash",
		"この動画を要約してください",
		[]gemini.Attachment{{URI: uploaded.URI, MIMEType: "video/mp4"}},
		gemini.GenerateOptions{})
	return err
}

func sampleVeo(ctx context.Context) error {
	client, err := gemini.NewClient(ctx, gemini.Config{ProjectID: "my-project", LocationID: "us-central1"})
	if err != nil {
		return err
	}
	videoClient, err := veo.New(client,
		veo.WithPollInterval(10*time.Second),
		veo.WithPollTimeout(15*time.Minute),
		veo.WithMaxPollErrors(10),
		veo.WithLogger(slog.Default()),
	)
	if err != nil {
		return err
	}
	result, err := videoClient.Generate(ctx, "veo-3.1-generate-001", veo.Request{
		Prompt:       "a slow dolly-in on a coastal cliffside at dawn",
		Image:        &veo.Media{URI: "gs://bucket/keyframe.png", MIMEType: "image/png"},
		DurationSec:  8,
		AspectRatio:  "16:9",
		OutputGCSURI: "gs://bucket/videos/",
	})
	if err != nil {
		return err
	}
	video, _ := result.First()
	_ = video.URI
	_, _, _ = result.OperationName, result.FilteredCount, result.FilteredReasons

	name, err := videoClient.Submit(ctx, "veo-3.1-generate-001", veo.Request{Prompt: "..."})
	if err != nil {
		return err
	}
	_, err = videoClient.Wait(ctx, name)
	return err
}

func sampleVeoRequestFields() {
	generateAudio := true
	seed := int64(1)
	_ = veo.Request{
		Prompt:         "p",
		DurationSec:    8,
		AspectRatio:    "16:9",
		Resolution:     "1080p",
		NegativePrompt: "blurry",
		GenerateAudio:  &generateAudio,
		Seed:           &seed,
		NumberOfVideos: 1,
		OutputGCSURI:   "gs://bucket/",
		LastFrame:      &veo.Media{URI: "gs://bucket/last.png"},
		Video:          &veo.Media{URI: "gs://bucket/prev.mp4"},
		References:     []veo.Reference{{Image: veo.Media{URI: "gs://bucket/c.png"}, Type: gemini.VideoReferenceAsset}},
	}
}

func sampleModifyRequestBody() {
	var req veo.Request
	req.ExtraBody = map[string]any{"parameters": map[string]any{"preview": true}}
	req.ModifyRequestBody = func(body map[string]any) map[string]any {
		instances, _ := body["instances"].([]any)
		if len(instances) > 0 {
			if instance, ok := instances[0].(map[string]any); ok {
				instance["audio"] = map[string]any{"gcsUri": "gs://bucket/bgm.mp3"}
			}
		}
		return body
	}
}

func sampleConfigFields() {
	_ = gemini.Config{
		APIKey:              "k",
		ProjectID:           "p",
		LocationID:          "l",
		MaxRetries:          1,
		InitialDelay:        30 * time.Second,
		MaxDelay:            120 * time.Second,
		FilePollingInterval: 2 * time.Second,
		FilePollingTimeout:  60 * time.Second,
		RequestTimeout:      5 * time.Minute,
		AsyncCleanupTimeout: 15 * time.Second,
		Logger:              slog.Default(),
		HTTPClient:          nil,
		OnRetry:             nil,
	}
}

func sampleGenerateOptions() {
	_ = gemini.GenerateOptions{SafetySettings: gemini.NewSafetySettings(gemini.SafetyBlockNone)}
	_ = gemini.GenerateOptions{
		ResponseMIMEType: "application/json",
		ResponseSchema: &gemini.Schema{
			Type: gemini.TypeObject,
			Properties: map[string]*gemini.Schema{
				"title":    {Type: gemini.TypeString},
				"keywords": {Type: gemini.TypeArray, Items: &gemini.Schema{Type: gemini.TypeString}},
			},
			Required: []string{"title"},
		},
	}
	// GenerateOptions の表に載せた全フィールド。
	_ = gemini.GenerateOptions{
		SystemPrompt:       "s",
		Temperature:        gemini.Ptr[float32](0),
		TopP:               gemini.Ptr[float32](0.9),
		TopK:               gemini.Ptr[float32](40),
		MaxOutputTokens:    2048,
		StopSequences:      []string{"END"},
		ThinkingBudget:     gemini.Ptr[int32](0),
		ThinkingLevel:      gemini.ThinkingLow,
		IncludeThoughts:    true,
		AspectRatio:        "16:9",
		ImageSize:          "1K",
		Seed:               gemini.Ptr[int64](1),
		PersonGeneration:   gemini.PersonGenerationAllowAll,
		ResponseJSONSchema: map[string]any{"type": "object"},
	}
}

// sampleConstantTables pins every constant listed in the README's "genai を import
// せずに値を選ぶ" table.
func sampleConstantTables() {
	_ = []gemini.SafetyThreshold{
		gemini.SafetyBlockNone, gemini.SafetyBlockLowAndAbove,
		gemini.SafetyBlockMediumAndAbove, gemini.SafetyBlockOnlyHigh, gemini.SafetyOff,
	}
	_ = []gemini.ThinkingLevel{
		gemini.ThinkingMinimal, gemini.ThinkingLow, gemini.ThinkingMedium,
		gemini.ThinkingHigh, gemini.ThinkingUnspecified,
	}
	_ = []gemini.PersonGeneration{
		gemini.PersonGenerationAllowAll, gemini.PersonGenerationAllowAdult,
		gemini.PersonGenerationDontAllow, gemini.PersonGenerationUnspecified,
	}
	_ = []gemini.SchemaType{
		gemini.TypeString, gemini.TypeNumber, gemini.TypeInteger,
		gemini.TypeBoolean, gemini.TypeArray, gemini.TypeObject,
	}
	_ = []gemini.VideoReferenceType{gemini.VideoReferenceAsset, gemini.VideoReferenceStyle}
}

func sampleErrorClassification(ctx context.Context, client *gemini.Client, model, prompt string) {
	_, err := client.GenerateContent(ctx, model, prompt)
	switch {
	case errors.Is(err, gemini.ErrBlocked):
		if apiErr, ok := errors.AsType[*gemini.APIResponseError](err); ok {
			slog.Warn("blocked", "reason", apiErr.FinishReason)
		}
	case errors.Is(err, gemini.ErrEmptyResponse):
	}
}

func sampleCleanJSON(resp *gemini.Response) error {
	var out readmeStruct
	jsonStr := gemini.CleanJSONResponse(resp.Text)
	return json.Unmarshal([]byte(jsonStr), &out)
}

func sampleMusicTypes() {
	var r music.Recipe
	r.Sections = []music.Section{{Name: "Verse", Duration: 30}}
	clone := r.Clone()
	_ = clone
}

// 参照されない関数を unused 検出から守るための参照点。
var _ = []any{
	sampleQuickstart, sampleMultimodal, sampleImageResponse, sampleFileAPI,
	sampleVeo, sampleVeoRequestFields, sampleModifyRequestBody, sampleConfigFields,
	sampleGenerateOptions, sampleConstantTables, sampleErrorClassification,
	sampleCleanJSON, sampleMusicTypes,
}
