package gemini

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/genai"
)

// UploadFile はデータをアップロードし、Active状態になるまでポーリングするのだ。
// 戻り値として、File APIでのURI、削除時に使用する名前、およびエラーを返すのだ。
func (c *Client) UploadFile(ctx context.Context, data []byte, mimeType, displayName string) (string, string, error) {
	reader := bytes.NewReader(data)
	uploadCfg := &genai.UploadFileConfig{
		MIMEType:    mimeType,
		DisplayName: displayName,
	}

	// 1. ファイルをアップロードするのだ
	file, err := c.client.Files.Upload(ctx, reader, uploadCfg)
	if err != nil {
		return "", "", fmt.Errorf("file upload failed: %w", err)
	}

	// 2. Active状態になるまでポーリング待機するのだ
	ticker := time.NewTicker(PollingInterval)
	defer ticker.Stop()

	// 無限ループを防ぐためのタイムアウト設定なのだ
	timeout := time.After(PollingTimeout)

	for {
		select {
		case <-ctx.Done():
			// 呼び出し元がキャンセルされた場合、後処理としてファイルの削除を試みる
			go func(fileName string) {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if _, err := c.client.Files.Delete(cleanupCtx, fileName, &genai.DeleteFileConfig{}); err != nil {
					slog.WarnContext(context.Background(), "Async cleanup of File API failed", "name", fileName, "error", err)
				}
			}(file.Name)
			return "", "", ctx.Err()

		case <-timeout:
			// タイムアウト発生時、ファイル名を含めた詳細なエラーを返しつつ、非同期で削除する
			go func(fileName string) {
				_, _ = c.client.Files.Delete(context.Background(), fileName, &genai.DeleteFileConfig{})
			}(file.Name)
			return "", "", fmt.Errorf("file processing for %q timed out after %v", file.Name, PollingTimeout)

		case <-ticker.C:
			// 現在の状態を取得するのだ
			currentFile, err := c.client.Files.Get(ctx, file.Name, &genai.GetFileConfig{})
			if err != nil {
				return "", "", fmt.Errorf("failed to get status for %q: %w", file.Name, err)
			}

			switch currentFile.State {
			case genai.FileStateActive:
				// 利用可能になったのだ！
				return currentFile.URI, currentFile.Name, nil
			case genai.FileStateFailed:
				// サーバー側で処理が失敗した場合
				return "", "", fmt.Errorf("File API processing failed on server side for %q", file.Name)
			case genai.FileStateProcessing:
				// まだ処理中なので次のループへ行くのだ
				slog.DebugContext(ctx, "File API processing...", "name", file.Name)
				continue
			default:
				// 未定義の状態などの場合
				slog.WarnContext(ctx, "Unknown file state received", "state", currentFile.State, "name", file.Name)
			}
		}
	}
}

// DeleteFile は指定された名前のファイルを File API から削除します。
func (c *Client) DeleteFile(ctx context.Context, fileName string) error {
	if fileName == "" {
		return nil
	}
	_, err := c.client.Files.Delete(ctx, fileName, &genai.DeleteFileConfig{})
	if err != nil {
		return fmt.Errorf("failed to delete file %q: %w", fileName, err)
	}
	slog.InfoContext(ctx, "File API object deleted", "name", fileName)
	return nil
}
