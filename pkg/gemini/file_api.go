package gemini

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/genai"
)

// UploadFile はデータをアップロードし、そのファイルが Active 状態（利用可能）になるまで待機します。
// 戻り値として、File API での URI、管理用のファイル名、およびエラーを返します。
func (c *Client) UploadFile(ctx context.Context, data []byte, mimeType, displayName string) (string, string, error) {
	uploadCfg := &genai.UploadFileConfig{
		MIMEType:    mimeType,
		DisplayName: displayName,
	}

	file, err := c.client.Files.Upload(ctx, bytes.NewReader(data), uploadCfg)
	if err != nil {
		return "", "", fmt.Errorf("failed to upload file to Gemini File API: %w", err)
	}

	// Active 状態になるのを待機
	uri, err := c.waitForFileActive(ctx, file.Name)
	if err != nil {
		// 失敗またはタイムアウトした場合は、リソースを非同期でクリーンアップする
		c.asyncDelete(file.Name)
		return "", "", err
	}

	return uri, file.Name, nil
}

// DeleteFile は指定された名前のファイルを File API から削除します。
func (c *Client) DeleteFile(ctx context.Context, fileName string) error {
	if fileName == "" {
		return nil
	}
	_, err := c.client.Files.Delete(ctx, fileName, nil)
	if err != nil {
		return fmt.Errorf("failed to delete file %q: %w", fileName, err)
	}
	slog.InfoContext(ctx, "File API object deleted", "name", fileName)
	return nil
}

// waitForFileActive は指定されたファイルが利用可能になるまでポーリングします。
func (c *Client) waitForFileActive(ctx context.Context, fileName string) (string, error) {
	ticker := time.NewTicker(PollingInterval)
	defer ticker.Stop()

	timeout := time.After(PollingTimeout)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()

		case <-timeout:
			return "", fmt.Errorf("processing for %q timed out after %v", fileName, PollingTimeout)

		case <-ticker.C:
			f, err := c.client.Files.Get(ctx, fileName, &genai.GetFileConfig{})
			if err != nil {
				return "", fmt.Errorf("failed to check file status: %w", err)
			}

			switch f.State {
			case genai.FileStateActive:
				return f.URI, nil
			case genai.FileStateFailed:
				return "", fmt.Errorf("server-side processing failed for file: %q", fileName)
			case genai.FileStateProcessing:
				slog.DebugContext(ctx, "Gemini File API processing...", "name", fileName)
			default:
				slog.WarnContext(ctx, "Unexpected file state received", "state", f.State, "name", fileName)
			}
		}
	}
}

// asyncDelete はエラー時などの後処理として、バックグラウンドでファイルを削除します。
func (c *Client) asyncDelete(fileName string) {
	go func() {
		// メインの context がキャンセルされていても実行できるよう、新しい context を作成
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := c.DeleteFile(ctx, fileName); err != nil {
			slog.WarnContext(context.Background(), "Failed to cleanup file asynchronously", "name", fileName, "error", err)
		}
	}()
}
