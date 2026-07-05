package gemini

import (
	"context"
	"errors"
	"io"
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
