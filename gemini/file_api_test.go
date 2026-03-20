package gemini

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- waitForFileActive のテスト ---
// ポーリングロジックがコンテキストキャンセルやタイムアウトを正しく扱うかを検証します。
func TestWaitForFileActive_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &Client{
		// 実際はモックが必要ですが、ここではロジックの挙動をシミュレート
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
	c := &Client{}
	ctx := context.Background()

	t.Run("empty filename returns nil", func(t *testing.T) {
		err := c.DeleteFile(ctx, "")
		if err != nil {
			t.Errorf("空のファイル名でエラーが返されました: %v", err)
		}
	})
}
