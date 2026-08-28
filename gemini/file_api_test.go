package gemini

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"google.golang.org/genai"
)

type fakeGetResult struct {
	file *genai.File
	err  error
}

type fakeFileClient struct {
	uploadFile *genai.File
	uploadErr  error
	getFiles   []*genai.File
	getErr     error
	// getResults は呼び出し順に返す結果です。getFiles / getErr より優先されます。
	getResults []fakeGetResult
	getCalls   int
	// getBlocks は、ステータス確認が応答を返さない状況を再現します。
	// context が終わるまで戻らないため、待機側の制限時間だけが打ち切り手段になります。
	getBlocks   bool
	deleteErr   error
	deleteCalls int
	// deleteSignal は非同期削除（asyncDelete）の完了をテストへ通知するためのチャネルです。
	// nil の場合は通知しません。deleteCalls のインクリメント後に送信するため、
	// 受信側は happens-before によりデータ競合なく deleteCalls を読み取れます。
	deleteSignal chan struct{}
}

func (f *fakeFileClient) Upload(_ context.Context, _ io.Reader, _ *genai.UploadFileConfig) (*genai.File, error) {
	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	if f.uploadFile != nil {
		return f.uploadFile, nil
	}
	return &genai.File{Name: "files/test"}, nil
}

func (f *fakeFileClient) Get(ctx context.Context, name string, _ *genai.GetFileConfig) (*genai.File, error) {
	f.getCalls++
	if f.getBlocks {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if len(f.getResults) > 0 {
		idx := f.getCalls - 1
		if idx >= len(f.getResults) {
			idx = len(f.getResults) - 1
		}
		r := f.getResults[idx]
		return r.file, r.err
	}
	if f.getErr != nil {
		return nil, f.getErr
	}
	if len(f.getFiles) == 0 {
		return &genai.File{Name: name, State: genai.FileStateProcessing}, nil
	}
	idx := f.getCalls - 1
	if idx >= len(f.getFiles) {
		idx = len(f.getFiles) - 1
	}
	return f.getFiles[idx], nil
}

func (f *fakeFileClient) Delete(_ context.Context, _ string, _ *genai.DeleteFileConfig) (*genai.DeleteFileResponse, error) {
	f.deleteCalls++
	if f.deleteSignal != nil {
		f.deleteSignal <- struct{}{}
	}
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &genai.DeleteFileResponse{}, nil
}

func TestWaitForFileActive_PollsUntilActive(t *testing.T) {
	// ポーリング間隔はバブル内の仮想時計で経過させます。実時間は消費しません。
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		fake := &fakeFileClient{
			getFiles: []*genai.File{
				{Name: "test-file", State: genai.FileStateProcessing},
				{Name: "test-file", URI: "https://example.com/file", State: genai.FileStateActive},
			},
		}
		client := &Client{
			fileClient:          fake,
			filePollingInterval: 2 * time.Second,
			filePollingTimeout:  time.Minute,
		}

		uri, err := client.waitForFileActive(ctx, "test-file")
		if err != nil {
			t.Fatalf("waitForFileActive() unexpected error = %v", err)
		}
		if uri != "https://example.com/file" {
			t.Fatalf("uri = %q, want https://example.com/file", uri)
		}
		if fake.getCalls != 2 {
			t.Fatalf("Get calls = %d, want 2", fake.getCalls)
		}
	})
}

// TestWaitForFileActive_TimeoutBoundsInFlightCheck は、応答を返さないステータス確認が
// 制限時間で打ち切られることを検証します。
//
// 制限時間をポーリングの合間でしか見張らない実装では、この待機は終わりません。
// 確認の中で止まっている間は時間切れの判定へ到達できず、genai の既定 HTTP クライアントにも
// タイムアウトが無いためです。
func TestWaitForFileActive_TimeoutBoundsInFlightCheck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeFileClient{getBlocks: true}
		client := &Client{
			fileClient:          fake,
			filePollingInterval: 2 * time.Second,
			filePollingTimeout:  10 * time.Second,
		}

		_, err := client.waitForFileActive(context.Background(), "test-file")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("waitForFileActive() error = %v, want a deadline error", err)
		}
		if !strings.Contains(err.Error(), "制限時間") {
			t.Errorf("エラーが時間切れであることを示していません: %v", err)
		}
		if fake.getCalls != 1 {
			t.Errorf("Get calls = %d, want 1 (確認は 1 回のまま打ち切られる)", fake.getCalls)
		}
	})
}

// --- UploadFile のオーケストレーションテスト ---
// Upload → waitForFileActive → 失敗時の asyncDelete という一連の流れを検証します。
func TestUploadFile_Success(t *testing.T) {
	ctx := context.Background()
	fake := &fakeFileClient{
		uploadFile: &genai.File{Name: "files/uploaded"},
		getFiles: []*genai.File{
			{Name: "files/uploaded", URI: "https://example.com/uploaded", State: genai.FileStateActive},
		},
	}
	client := &Client{
		fileClient:          fake,
		filePollingInterval: time.Hour,
		filePollingTimeout:  time.Hour,
	}

	got, err := client.UploadFile(ctx, strings.NewReader("data"), "text/plain", "display")
	if err != nil {
		t.Fatalf("UploadFile() unexpected error = %v", err)
	}
	if got.URI != "https://example.com/uploaded" {
		t.Fatalf("URI = %q, want https://example.com/uploaded", got.URI)
	}
	if got.Name != "files/uploaded" {
		t.Fatalf("Name = %q, want files/uploaded", got.Name)
	}
	if fake.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0 (成功時はクリーンアップしない)", fake.deleteCalls)
	}
}

func TestUploadFile_UploadError(t *testing.T) {
	ctx := context.Background()
	fake := &fakeFileClient{
		uploadErr: errors.New("boom"),
	}
	client := &Client{
		fileClient:          fake,
		filePollingInterval: time.Hour,
		filePollingTimeout:  time.Hour,
	}

	_, err := client.UploadFile(ctx, strings.NewReader("data"), "text/plain", "display")
	if err == nil {
		t.Fatal("アップロード失敗時にエラーが返されませんでした")
	}
	if fake.getCalls != 0 {
		t.Fatalf("Get calls = %d, want 0 (アップロード失敗時は状態確認しない)", fake.getCalls)
	}
	if fake.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0 (アップロード失敗時はクリーンアップしない)", fake.deleteCalls)
	}
}

func TestUploadFile_WaitFailsTriggersCleanup(t *testing.T) {
	// 連続失敗の上限に達するまでのポーリングも、クリーンアップの完了待ちも
	// バブル内の仮想時計で進みます。実時間は消費しません。
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		fake := &fakeFileClient{
			uploadFile:   &genai.File{Name: "files/uploaded"},
			getErr:       errors.New("status check failed"),
			deleteSignal: make(chan struct{}, 1),
		}
		client := &Client{
			fileClient:          fake,
			filePollingInterval: 2 * time.Second,
			filePollingTimeout:  time.Minute,
		}

		_, err := client.UploadFile(ctx, strings.NewReader("data"), "text/plain", "display")
		if err == nil {
			t.Fatal("Active 化失敗時にエラーが返されませんでした")
		}
		if !strings.Contains(err.Error(), "連続で失敗") {
			t.Fatalf("連続失敗の打ち切りエラーを期待しましたが: %v", err)
		}
		if fake.getCalls != fileStateMaxPollErrors {
			t.Fatalf("Get calls = %d, want %d (許容回数まで再確認する)", fake.getCalls, fileStateMaxPollErrors)
		}

		// asyncDelete はバックグラウンドで実行されるため、削除の完了を待つ。
		select {
		case <-fake.deleteSignal:
		case <-time.After(AsyncCleanupTimeout):
			t.Fatal("asyncDelete によるクリーンアップが実行されませんでした")
		}
		if fake.deleteCalls != 1 {
			t.Fatalf("delete calls = %d, want 1 (待機失敗時はクリーンアップが1回発火する)", fake.deleteCalls)
		}
	})
}

// --- checkFileState の失敗系ブランチのテスト ---
func TestCheckFileState_Failed(t *testing.T) {
	ctx := context.Background()
	fake := &fakeFileClient{
		getFiles: []*genai.File{
			{Name: "test-file", State: genai.FileStateFailed},
		},
	}
	client := &Client{fileClient: fake}

	uri, done, err := client.checkFileState(ctx, "test-file")
	if err == nil {
		t.Fatal("FileStateFailed でエラーが返されませんでした")
	}
	if !done {
		t.Fatal("FileStateFailed は done=true であるべきです")
	}
	if uri != "" {
		t.Fatalf("uri = %q, want empty", uri)
	}
}

func TestCheckFileState_GetError(t *testing.T) {
	ctx := context.Background()
	fake := &fakeFileClient{
		getErr: errors.New("network down"),
	}
	client := &Client{fileClient: fake}

	_, done, err := client.checkFileState(ctx, "test-file")
	if err == nil {
		t.Fatal("Get 失敗時にエラーが返されませんでした")
	}
	if done {
		t.Fatal("Get エラー時は done=false であるべきです")
	}
	if !strings.Contains(err.Error(), "ステータス確認に失敗") {
		t.Fatalf("エラーがラップされていません: %v", err)
	}
}

// --- DeleteFile の成功/失敗テスト ---
func TestDeleteFile_Success(t *testing.T) {
	ctx := context.Background()
	fake := &fakeFileClient{}
	client := &Client{fileClient: fake}

	if err := client.DeleteFile(ctx, "files/to-delete"); err != nil {
		t.Fatalf("DeleteFile() unexpected error = %v", err)
	}
	if fake.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", fake.deleteCalls)
	}
}

func TestDeleteFile_Error(t *testing.T) {
	ctx := context.Background()
	fake := &fakeFileClient{deleteErr: errors.New("delete failed")}
	client := &Client{fileClient: fake}

	err := client.DeleteFile(ctx, "files/to-delete")
	if err == nil {
		t.Fatal("削除失敗時にエラーが返されませんでした")
	}
	if !strings.Contains(err.Error(), "削除に失敗") {
		t.Fatalf("エラーがラップされていません: %v", err)
	}
}

// --- asyncDelete のテスト ---
func TestAsyncDelete(t *testing.T) {
	// このテストはパニックが起きないこと、および
	// 非同期実行が正常に開始されることを確認します。
	c := &Client{logger: slog.Default(), asyncCleanupTimeout: AsyncCleanupTimeout}

	// 空の名前でも安全に終了するか
	t.Run("empty filename should not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("asyncDelete panicked with empty filename: %v", r)
			}
		}()
		c.asyncDelete(context.Background(), "")
	})
}

// --- DeleteFile のバリデーションテスト ---
func TestDeleteFile_Validation(t *testing.T) {
	c := &Client{fileClient: &fakeFileClient{}}
	ctx := context.Background()

	t.Run("empty filename returns nil", func(t *testing.T) {
		err := c.DeleteFile(ctx, "")
		if err != nil {
			t.Errorf("空のファイル名でエラーが返されました: %v", err)
		}
	})
}
