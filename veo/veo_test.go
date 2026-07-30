package veo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shouni/go-gemini-client/gemini"
)

// fakeGenerator は gemini.VideoGenerator のテストダブルです。genai SDK も GCP 認証も
// 使わずにポーリングの挙動を検証できることが、DI で境界を切っている利点そのものです。
type fakeGenerator struct {
	startOp  *gemini.VideoOperation
	startErr error

	// polls は PollVideo が返す応答を順に消費します。最後の要素に到達したら
	// 以降はそれを返し続けます。
	polls     []pollResponse
	pollCalls int
	lastName  string
}

type pollResponse struct {
	op  *gemini.VideoOperation
	err error
}

func (f *fakeGenerator) StartVideo(_ context.Context, _ string, _ gemini.VideoRequest) (*gemini.VideoOperation, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	return f.startOp, nil
}

func (f *fakeGenerator) PollVideo(_ context.Context, operationName string) (*gemini.VideoOperation, error) {
	f.lastName = operationName
	i := f.pollCalls
	f.pollCalls++
	if i >= len(f.polls) {
		i = len(f.polls) - 1
	}
	return f.polls[i].op, f.polls[i].err
}

// newTestClient は、テストが待たされないよう極小のポーリング間隔で Client を作ります。
func newTestClient(t *testing.T, generator gemini.VideoGenerator, opts ...Option) *Client {
	t.Helper()
	base := []Option{WithPollInterval(time.Millisecond), WithPollTimeout(2 * time.Second)}
	c, err := New(generator, append(base, opts...)...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return c
}

func running(name string) *gemini.VideoOperation {
	return &gemini.VideoOperation{Name: name}
}

func finished(name string, uris ...string) *gemini.VideoOperation {
	op := &gemini.VideoOperation{Name: name, Done: true}
	for _, uri := range uris {
		op.Videos = append(op.Videos, gemini.Attachment{URI: uri, MIMEType: "video/mp4"})
	}
	return op
}

func TestNewRequiresGenerator(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, ErrGeneratorRequired) {
		t.Fatalf("New(nil) error = %v, want ErrGeneratorRequired", err)
	}
}

// TestGeneratePollsUntilDone は、投函後に完了するまでポーリングし、完了した時点の
// 結果を返すことを検証します。
func TestGeneratePollsUntilDone(t *testing.T) {
	generator := &fakeGenerator{
		startOp: running("operations/abc"),
		polls: []pollResponse{
			{op: running("operations/abc")},
			{op: running("operations/abc")},
			{op: finished("operations/abc", "gs://bucket/out.mp4")},
		},
	}
	client := newTestClient(t, generator)

	got, err := client.Generate(context.Background(), "veo-3.1-generate-001", Request{Prompt: "a cat"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got.OperationName != "operations/abc" {
		t.Errorf("OperationName = %q", got.OperationName)
	}
	if generator.lastName != "operations/abc" {
		t.Errorf("polled name = %q, want the started operation's name", generator.lastName)
	}
	video, ok := got.First()
	if !ok || video.URI != "gs://bucket/out.mp4" {
		t.Errorf("First() = %+v, %v", video, ok)
	}
	if generator.pollCalls != 3 {
		t.Errorf("poll calls = %d, want 3", generator.pollCalls)
	}
}

// TestGenerateReturnsImmediatelyWhenAlreadyDone は、投函の応答が既に完了していた場合に
// 1度もポーリングせず結果を返すことを検証します。
func TestGenerateReturnsImmediatelyWhenAlreadyDone(t *testing.T) {
	generator := &fakeGenerator{startOp: finished("operations/done", "gs://bucket/out.mp4")}
	client := newTestClient(t, generator)

	if _, err := client.Generate(context.Background(), "veo-3.1-generate-001", Request{Prompt: "a cat"}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if generator.pollCalls != 0 {
		t.Errorf("poll calls = %d, want 0", generator.pollCalls)
	}
}

// TestGeneratePropagatesStartError は、投函に失敗したらポーリングへ進まないことを
// 検証します。リトライは gemini 側で既に尽きているため、ここで再試行はしません。
func TestGeneratePropagatesStartError(t *testing.T) {
	sentinel := errors.New("quota exceeded")
	generator := &fakeGenerator{startErr: sentinel}
	client := newTestClient(t, generator)

	_, err := client.Generate(context.Background(), "veo-3.1-generate-001", Request{Prompt: "a cat"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Generate() error = %v, want the start error", err)
	}
	if generator.pollCalls != 0 {
		t.Errorf("poll calls = %d, want 0", generator.pollCalls)
	}
}

// TestWaitToleratesTransientPollErrors は、一時的な確認失敗を受け流して待ち続け、
// 成功したら連続失敗のカウントが戻ることを検証します。生成済みの動画を一時的な
// ネットワーク障害で取り逃がさないための挙動です。
func TestWaitToleratesTransientPollErrors(t *testing.T) {
	generator := &fakeGenerator{
		polls: []pollResponse{
			{err: errors.New("temporary failure")},
			{err: errors.New("temporary failure")},
			{op: running("operations/abc")}, // ここでカウントがリセットされる
			{err: errors.New("temporary failure")},
			{err: errors.New("temporary failure")},
			{op: finished("operations/abc", "gs://bucket/out.mp4")},
		},
	}
	client := newTestClient(t, generator, WithMaxPollErrors(3))

	got, err := client.Wait(context.Background(), "operations/abc")
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if _, ok := got.First(); !ok {
		t.Fatal("expected a video in the result")
	}
}

// TestWaitStopsAfterConsecutivePollErrors は、確認が続けて失敗したらタイムアウトまで
// 粘らずに打ち切ることを検証します。原因は Unwrap で辿れます。
func TestWaitStopsAfterConsecutivePollErrors(t *testing.T) {
	sentinel := errors.New("permission denied")
	generator := &fakeGenerator{polls: []pollResponse{{err: sentinel}}}
	client := newTestClient(t, generator, WithMaxPollErrors(3))

	_, err := client.Wait(context.Background(), "operations/abc")
	if !errors.Is(err, ErrPollFailed) {
		t.Fatalf("Wait() error = %v, want ErrPollFailed", err)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Wait() error = %v, want the underlying cause to be reachable", err)
	}
	if generator.pollCalls != 3 {
		t.Errorf("poll calls = %d, want to stop at the limit of 3", generator.pollCalls)
	}
}

// TestWaitTimesOut は、生成が終わらないまま上限時間に達した場合に打ち切ることを
// 検証します。
func TestWaitTimesOut(t *testing.T) {
	generator := &fakeGenerator{polls: []pollResponse{{op: running("operations/abc")}}}
	client := newTestClient(t, generator, WithPollTimeout(20*time.Millisecond))

	_, err := client.Wait(context.Background(), "operations/abc")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait() error = %v, want a deadline error", err)
	}
	if errors.Is(err, ErrPollFailed) {
		t.Error("timing out is not a polling failure; it should not report ErrPollFailed")
	}
}

// TestWaitStopsWhenCallerCancels は、呼び出し側の context が終了したら即座に
// 中断することを検証します。
func TestWaitStopsWhenCallerCancels(t *testing.T) {
	generator := &fakeGenerator{polls: []pollResponse{{op: running("operations/abc")}}}
	client := newTestClient(t, generator)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Wait(ctx, "operations/abc")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v, want context.Canceled", err)
	}
}

// TestWaitPropagatesGenerationFailure は、生成そのものが失敗として完了した場合に、
// 通信エラーではなく生成失敗として返すことを検証します。
func TestWaitPropagatesGenerationFailure(t *testing.T) {
	failed := &gemini.VideoOperation{
		Name:    "operations/abc",
		Done:    true,
		Failure: fmt.Errorf("%w: code=3: INVALID_ARGUMENT", gemini.ErrVideoGenerationFailed),
	}
	generator := &fakeGenerator{polls: []pollResponse{{op: failed}}}
	client := newTestClient(t, generator)

	_, err := client.Wait(context.Background(), "operations/abc")
	if !errors.Is(err, gemini.ErrVideoGenerationFailed) {
		t.Fatalf("Wait() error = %v, want ErrVideoGenerationFailed", err)
	}
}

// TestWaitReportsSafetyFiltering は、成功で完了したのに動画が無い場合に、除外の
// 理由まで含めて報告することを検証します。プロンプトを直すのに必要な情報です。
func TestWaitReportsSafetyFiltering(t *testing.T) {
	filtered := &gemini.VideoOperation{
		Name:            "operations/abc",
		Done:            true,
		FilteredCount:   1,
		FilteredReasons: []string{"violence"},
	}
	generator := &fakeGenerator{polls: []pollResponse{{op: filtered}}}
	client := newTestClient(t, generator)

	_, err := client.Wait(context.Background(), "operations/abc")
	if !errors.Is(err, ErrNoVideoGenerated) {
		t.Fatalf("Wait() error = %v, want ErrNoVideoGenerated", err)
	}
	if got := err.Error(); !strings.Contains(got, "violence") {
		t.Errorf("error = %q, want it to name the filter reason", got)
	}
}

func TestWaitRequiresOperationName(t *testing.T) {
	client := newTestClient(t, &fakeGenerator{})
	if _, err := client.Wait(context.Background(), "  "); !errors.Is(err, ErrMissingOperationName) {
		t.Fatalf("Wait(blank) error = %v, want ErrMissingOperationName", err)
	}
}
