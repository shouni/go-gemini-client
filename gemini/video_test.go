package gemini

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

// fakeVideoClient は videoClient のテストダブルで、SDK へ渡された引数を記録します。
type fakeVideoClient struct {
	gotModel  string
	gotSource *genai.GenerateVideosSource
	gotConfig *genai.GenerateVideosConfig
	gotPollOp *genai.GenerateVideosOperation

	startOp  *genai.GenerateVideosOperation
	startErr error
	pollOp   *genai.GenerateVideosOperation
	pollErr  error
	calls    int
}

func (f *fakeVideoClient) GenerateVideosFromSource(_ context.Context, model string, source *genai.GenerateVideosSource, config *genai.GenerateVideosConfig) (*genai.GenerateVideosOperation, error) {
	f.calls++
	f.gotModel, f.gotSource, f.gotConfig = model, source, config
	if f.startErr != nil {
		return nil, f.startErr
	}
	if f.startOp != nil {
		return f.startOp, nil
	}
	return &genai.GenerateVideosOperation{Name: "operations/abc"}, nil
}

func (f *fakeVideoClient) GetVideosOperation(_ context.Context, operation *genai.GenerateVideosOperation, _ *genai.GetOperationConfig) (*genai.GenerateVideosOperation, error) {
	f.calls++
	f.gotPollOp = operation
	return f.pollOp, f.pollErr
}

func newVideoTestClient(video *fakeVideoClient) *Client {
	return &Client{videoClient: video, retryOpts: Config{}.buildRetryOptions()}
}

// TestStartVideoBuildsImageToVideoRequest は、開始フレームと生成パラメータが SDK の
// 入力へ正しく変換されることを検証します。
func TestStartVideoBuildsImageToVideoRequest(t *testing.T) {
	video := &fakeVideoClient{}
	client := newVideoTestClient(video)
	seed := int64(4242)
	generateAudio := true

	op, err := client.StartVideo(context.Background(), "veo-3.1-generate-001", VideoRequest{
		Prompt:         "slow dolly in",
		Image:          &Attachment{URI: "gs://bucket/kf.png", MIMEType: "image/png"},
		DurationSec:    8,
		Seed:           &seed,
		AspectRatio:    "16:9",
		Resolution:     "1080p",
		NegativePrompt: "text, watermark",
		GenerateAudio:  &generateAudio,
		OutputGCSURI:   "gs://bucket/out/",
		NumberOfVideos: 1,
	})
	if err != nil {
		t.Fatalf("StartVideo() error = %v", err)
	}
	if op.Name != "operations/abc" || op.Done {
		t.Errorf("operation = %+v", op)
	}
	if video.gotModel != "veo-3.1-generate-001" {
		t.Errorf("model = %q", video.gotModel)
	}
	if video.gotSource.Prompt != "slow dolly in" {
		t.Errorf("prompt = %q", video.gotSource.Prompt)
	}
	if video.gotSource.Image == nil || video.gotSource.Image.GCSURI != "gs://bucket/kf.png" {
		t.Errorf("image = %+v", video.gotSource.Image)
	}
	if video.gotConfig.DurationSeconds == nil || *video.gotConfig.DurationSeconds != 8 {
		t.Errorf("durationSeconds = %v", video.gotConfig.DurationSeconds)
	}
	if video.gotConfig.Seed == nil || *video.gotConfig.Seed != 4242 {
		t.Errorf("seed = %v", video.gotConfig.Seed)
	}
	if video.gotConfig.OutputGCSURI != "gs://bucket/out/" || video.gotConfig.AspectRatio != "16:9" {
		t.Errorf("config = %+v", video.gotConfig)
	}
	if video.gotConfig.Resolution != "1080p" || video.gotConfig.NegativePrompt != "text, watermark" {
		t.Errorf("config = %+v", video.gotConfig)
	}
	if video.gotConfig.GenerateAudio == nil || !*video.gotConfig.GenerateAudio {
		t.Errorf("generateAudio = %v", video.gotConfig.GenerateAudio)
	}
}

// TestStartVideoBuildsReferenceImages は、参照画像が種別付きで SDK へ渡ることを
// 検証します。
func TestStartVideoBuildsReferenceImages(t *testing.T) {
	video := &fakeVideoClient{}
	client := newVideoTestClient(video)

	_, err := client.StartVideo(context.Background(), "veo-3.1-generate-001", VideoRequest{
		Prompt: "a character walking",
		References: []VideoReference{
			{Image: Attachment{URI: "gs://bucket/char.png"}, Type: VideoReferenceAsset},
			{Image: Attachment{}}, // 空の参照は落とす
			{Image: Attachment{URI: "gs://bucket/style.png"}, Type: VideoReferenceStyle},
		},
	})
	if err != nil {
		t.Fatalf("StartVideo() error = %v", err)
	}
	refs := video.gotConfig.ReferenceImages
	if len(refs) != 2 {
		t.Fatalf("referenceImages = %d, want 2 (empty entries dropped)", len(refs))
	}
	if refs[0].Image.GCSURI != "gs://bucket/char.png" || refs[0].ReferenceType != VideoReferenceAsset {
		t.Errorf("references[0] = %+v", refs[0])
	}
	if refs[1].Image.GCSURI != "gs://bucket/style.png" || refs[1].ReferenceType != VideoReferenceStyle {
		t.Errorf("references[1] = %+v", refs[1])
	}
}

// TestStartVideoBuildsVideoExtension は、継続生成の入力動画が SDK へ渡ることを
// 検証します。
func TestStartVideoBuildsVideoExtension(t *testing.T) {
	video := &fakeVideoClient{}
	client := newVideoTestClient(video)

	_, err := client.StartVideo(context.Background(), "veo-3.1-generate-001", VideoRequest{
		Prompt: "continue the motion",
		Video:  &Attachment{URI: "gs://bucket/prev.mp4", MIMEType: "video/mp4"},
	})
	if err != nil {
		t.Fatalf("StartVideo() error = %v", err)
	}
	if video.gotSource.Video == nil || video.gotSource.Video.URI != "gs://bucket/prev.mp4" {
		t.Errorf("video = %+v", video.gotSource.Video)
	}
	if video.gotSource.Image != nil {
		t.Errorf("image = %+v, want none", video.gotSource.Image)
	}
}

// TestStartVideoPassesExtraBody は、SDK が型として持たないフィールドを ExtraBody で
// 送れることを検証します。プレビュー機能のために生 REST へ戻らずに済む逃げ道です。
func TestStartVideoPassesExtraBody(t *testing.T) {
	video := &fakeVideoClient{}
	client := newVideoTestClient(video)

	_, err := client.StartVideo(context.Background(), "veo-3.1-generate-001", VideoRequest{
		Prompt:    "a cat",
		ExtraBody: map[string]any{"instances": []any{map[string]any{"audio": map[string]any{"gcsUri": "gs://bucket/bgm.mp3"}}}},
	})
	if err != nil {
		t.Fatalf("StartVideo() error = %v", err)
	}
	if video.gotConfig.HTTPOptions == nil || video.gotConfig.HTTPOptions.ExtraBody == nil {
		t.Fatalf("httpOptions = %+v, want ExtraBody to be forwarded", video.gotConfig.HTTPOptions)
	}
}

// TestStartVideoRejectsInvalidInputCombinations は、API が受け付けない入力の
// 組み合わせを送信前に弾くことを検証します。Veo は video と image / referenceImages を
// 併用できず、lastFrame は image とセットでのみ有効です。
func TestStartVideoRejectsInvalidInputCombinations(t *testing.T) {
	image := &Attachment{URI: "gs://bucket/kf.png"}
	tests := []struct {
		name string
		req  VideoRequest
		want error
	}{
		{
			name: "image and video together",
			req:  VideoRequest{Prompt: "p", Image: image, Video: &Attachment{URI: "gs://bucket/prev.mp4"}},
			want: ErrInvalidVideoInput,
		},
		{
			name: "references with an image",
			req:  VideoRequest{Prompt: "p", Image: image, References: []VideoReference{{Image: Attachment{URI: "gs://bucket/char.png"}}}},
			want: ErrInvalidVideoInput,
		},
		{
			name: "last frame without a start image",
			req:  VideoRequest{Prompt: "p", LastFrame: &Attachment{URI: "gs://bucket/next.png"}},
			want: ErrInvalidVideoInput,
		},
		{
			name: "attachment with both data and uri",
			req:  VideoRequest{Prompt: "p", Image: &Attachment{URI: "gs://bucket/kf.png", Data: []byte{0x89}}},
			want: ErrInvalidVideoInput,
		},
		{
			name: "no prompt and no media",
			req:  VideoRequest{},
			want: ErrEmptyPrompt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			video := &fakeVideoClient{}
			client := newVideoTestClient(video)

			_, err := client.StartVideo(context.Background(), "veo-3.1-generate-001", tt.req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("StartVideo() error = %v, want %v", err, tt.want)
			}
			if video.calls != 0 {
				t.Errorf("SDK calls = %d, want the request rejected before sending", video.calls)
			}
		})
	}
}

// TestStartVideoValidatesSeedRange は、int32 に収まらないシードを送信前に弾くことを
// 検証します（SDK のシードは int32）。
func TestStartVideoValidatesSeedRange(t *testing.T) {
	video := &fakeVideoClient{}
	client := newVideoTestClient(video)
	seed := int64(1) << 40

	_, err := client.StartVideo(context.Background(), "veo-3.1-generate-001", VideoRequest{Prompt: "p", Seed: &seed})
	if !errors.Is(err, ErrInvalidSeed) {
		t.Fatalf("StartVideo() error = %v, want ErrInvalidSeed", err)
	}
}

func TestStartVideoRequiresModelName(t *testing.T) {
	client := newVideoTestClient(&fakeVideoClient{})
	if _, err := client.StartVideo(context.Background(), "", VideoRequest{Prompt: "p"}); !errors.Is(err, ErrEmptyModelName) {
		t.Fatalf("StartVideo() error = %v, want ErrEmptyModelName", err)
	}
}

// TestPollVideoMapsCompletedOperation は、完了したオペレーションの生成結果と
// 安全性フィルタの情報が公開型へ移されることを検証します。
func TestPollVideoMapsCompletedOperation(t *testing.T) {
	video := &fakeVideoClient{
		pollOp: &genai.GenerateVideosOperation{
			Name: "operations/abc",
			Done: true,
			Response: &genai.GenerateVideosResponse{
				GeneratedVideos: []*genai.GeneratedVideo{
					{Video: &genai.Video{URI: "gs://bucket/out.mp4", MIMEType: "video/mp4"}},
					nil, // 欠けた要素があっても落とさない
				},
				RAIMediaFilteredCount:   1,
				RAIMediaFilteredReasons: []string{"violence"},
			},
		},
	}
	client := newVideoTestClient(video)

	op, err := client.PollVideo(context.Background(), "operations/abc")
	if err != nil {
		t.Fatalf("PollVideo() error = %v", err)
	}
	if video.gotPollOp == nil || video.gotPollOp.Name != "operations/abc" {
		t.Errorf("polled operation = %+v", video.gotPollOp)
	}
	if !op.Done || len(op.Videos) != 1 || op.Videos[0].URI != "gs://bucket/out.mp4" {
		t.Errorf("operation = %+v", op)
	}
	if op.Videos[0].MIMEType != "video/mp4" {
		t.Errorf("mime type = %q", op.Videos[0].MIMEType)
	}
	if op.FilteredCount != 1 || len(op.FilteredReasons) != 1 {
		t.Errorf("filtered = %d %v", op.FilteredCount, op.FilteredReasons)
	}
	if op.Failure != nil {
		t.Errorf("failure = %v, want none", op.Failure)
	}
}

// TestPollVideoMapsOperationFailure は、失敗として完了したオペレーションが
// ErrVideoGenerationFailed で判定できる error になることを検証します。
// 取得自体は成功しているため、PollVideo は error を返しません。
func TestPollVideoMapsOperationFailure(t *testing.T) {
	video := &fakeVideoClient{
		pollOp: &genai.GenerateVideosOperation{
			Name: "operations/abc",
			Done: true,
			Error: map[string]any{
				"code":    float64(3),
				"status":  "INVALID_ARGUMENT",
				"message": "Video duration 36 seconds exceeds the maximum duration 30 seconds",
			},
		},
	}
	client := newVideoTestClient(video)

	op, err := client.PollVideo(context.Background(), "operations/abc")
	if err != nil {
		t.Fatalf("PollVideo() error = %v, want the failure on the operation instead", err)
	}
	if !errors.Is(op.Failure, ErrVideoGenerationFailed) {
		t.Fatalf("failure = %v, want ErrVideoGenerationFailed", op.Failure)
	}
	for _, want := range []string{"code=3", "INVALID_ARGUMENT", "exceeds the maximum duration"} {
		if !contains(op.Failure.Error(), want) {
			t.Errorf("failure = %q, want it to contain %q", op.Failure.Error(), want)
		}
	}
}

func TestPollVideoRequiresOperationName(t *testing.T) {
	client := newVideoTestClient(&fakeVideoClient{})
	if _, err := client.PollVideo(context.Background(), "  "); !errors.Is(err, ErrEmptyOperationName) {
		t.Fatalf("PollVideo() error = %v, want ErrEmptyOperationName", err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
