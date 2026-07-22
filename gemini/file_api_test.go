package gemini

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"
)

type fakeFileClient struct {
	uploadFile  *genai.File
	uploadErr   error
	getFiles    []*genai.File
	getErr      error
	getCalls    int
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

func (f *fakeFileClient) Get(_ context.Context, name string, _ *genai.GetFileConfig) (*genai.File, error) {
	f.getCalls++
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

// --- waitForFileActive のテスト ---
// ポーリングロジックがコンテキストキャンセルやタイムアウトを正しく扱うかを検証します。
func TestWaitForFileActive_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeFileClient{}
	client := &Client{
		fileClient:          fake,
		filePollingInterval: 10 * time.Millisecond,
		filePollingTimeout:  time.Second,
	}

	// 実行後すぐにキャンセル
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := client.waitForFileActive(ctx, "test-file")
	if err == nil {
		t.Error("コンテキストがキャンセルされたのにエラーが返されませんでした")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("エラーが context.Canceled ではありません: %v", err)
	}
}

func TestWaitForFileActive_ImmediateActive(t *testing.T) {
	ctx := context.Background()
	fake := &fakeFileClient{
		getFiles: []*genai.File{
			{Name: "test-file", URI: "https://example.com/file", State: genai.FileStateActive},
		},
	}
	client := &Client{
		fileClient:          fake,
		filePollingInterval: time.Hour,
		filePollingTimeout:  time.Hour,
	}

	uri, err := client.waitForFileActive(ctx, "test-file")
	if err != nil {
		t.Fatalf("waitForFileActive() unexpected error = %v", err)
	}
	if uri != "https://example.com/file" {
		t.Fatalf("uri = %q, want https://example.com/file", uri)
	}
	if fake.getCalls != 1 {
		t.Fatalf("Get calls = %d, want 1", fake.getCalls)
	}
}

func TestWaitForFileActive_PollsUntilActive(t *testing.T) {
	ctx := context.Background()
	fake := &fakeFileClient{
		getFiles: []*genai.File{
			{Name: "test-file", State: genai.FileStateProcessing},
			{Name: "test-file", URI: "https://example.com/file", State: genai.FileStateActive},
		},
	}
	client := &Client{
		fileClient:          fake,
		filePollingInterval: time.Millisecond,
		filePollingTimeout:  time.Second,
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

	uri, name, err := client.UploadFile(ctx, strings.NewReader("data"), "text/plain", "display")
	if err != nil {
		t.Fatalf("UploadFile() unexpected error = %v", err)
	}
	if uri != "https://example.com/uploaded" {
		t.Fatalf("uri = %q, want https://example.com/uploaded", uri)
	}
	if name != "files/uploaded" {
		t.Fatalf("name = %q, want files/uploaded", name)
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

	_, _, err := client.UploadFile(ctx, strings.NewReader("data"), "text/plain", "display")
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
	ctx := context.Background()
	fake := &fakeFileClient{
		uploadFile:   &genai.File{Name: "files/uploaded"},
		getErr:       errors.New("status check failed"),
		deleteSignal: make(chan struct{}, 1),
	}
	client := &Client{
		fileClient:          fake,
		filePollingInterval: time.Hour,
		filePollingTimeout:  time.Hour,
	}

	_, _, err := client.UploadFile(ctx, strings.NewReader("data"), "text/plain", "display")
	if err == nil {
		t.Fatal("Active 化失敗時にエラーが返されませんでした")
	}

	// asyncDelete はバックグラウンドで実行されるため、削除の完了を待つ。
	select {
	case <-fake.deleteSignal:
	case <-time.After(2 * time.Second):
		t.Fatal("asyncDelete によるクリーンアップが実行されませんでした")
	}
	if fake.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1 (待機失敗時はクリーンアップが1回発火する)", fake.deleteCalls)
	}
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
	c := &Client{}

	// 空の名前でも安全に終了するか
	t.Run("empty filename should not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("asyncDelete panicked with empty filename: %v", r)
			}
		}()
		c.asyncDelete("")
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
