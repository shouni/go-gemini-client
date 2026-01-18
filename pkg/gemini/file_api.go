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
		return "", "", fmt.Errorf("Gemini File API へのアップロードに失敗しました: %w", err)
	}

	// Active 状態になるのを待機
	uri, err := c.waitForFileActive(ctx, file.Name)
	if err != nil {
		// 失敗またはタイムアウトした場合は、リソースを非同期でクリーンアップする
		c.asyncDelete(file.Name)
		return "", "", fmt.Errorf("ファイル %q が有効状態になるまでの待機中にエラーが発生しました: %w", file.Name, err)
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
		return fmt.Errorf("ファイル %q の削除に失敗しました: %w", fileName, err)
	}
	slog.InfoContext(ctx, "File API オブジェクトを削除しました", "name", fileName)
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
			return "", fmt.Errorf("ファイル %q の待機中にコンテキストがキャンセルされました: %w", fileName, ctx.Err())

		case <-timeout:
			return "", fmt.Errorf("ファイル %q の処理が制限時間（%v）内に完了しませんでした", fileName, PollingTimeout)

		case <-ticker.C:
			f, err := c.client.Files.Get(ctx, fileName, &genai.GetFileConfig{})
			if err != nil {
				return "", fmt.Errorf("ファイル %q のステータス確認に失敗しました: %w", fileName, err)
			}

			switch f.State {
			case genai.FileStateActive:
				return f.URI, nil
			case genai.FileStateFailed:
				return "", fmt.Errorf("サーバー側でのファイル処理に失敗しました: %q", fileName)
			case genai.FileStateProcessing:
				slog.DebugContext(ctx, "Gemini File API で処理中...", "name", fileName)
			default:
				slog.WarnContext(ctx, "予期しないファイルステータスを受信しました", "state", f.State, "name", fileName)
			}
		}
	}
}

// asyncDelete はエラー時などの後処理として、バックグラウンドでファイルを削除します。
func (c *Client) asyncDelete(fileName string) {
	go func() {
		// メインの context がキャンセルされていても実行できるよう、新しい context を作成
		ctx, cancel := context.WithTimeout(context.Background(), AsyncCleanupTimeout)
		defer cancel()
		if err := c.DeleteFile(ctx, fileName); err != nil {
			slog.WarnContext(context.Background(), "バックグラウンドでのファイルクリーンアップに失敗しました", "name", fileName, "error", err)
		}
	}()
}
