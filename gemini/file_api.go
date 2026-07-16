package gemini

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"google.golang.org/genai"
)

// UploadFile はデータをアップロードし、そのファイルが Active 状態になるまで待機します。
// アップロード処理自体が成功した場合、たとえその後の Active 化処理でエラーが発生しても
// サーバー側にリソースが残る可能性があるため、バックグラウンドでの削除を試みます。
func (c *Client) UploadFile(ctx context.Context, r io.Reader, mimeType, displayName string) (string, string, error) {
	uploadCfg := &genai.UploadFileConfig{
		MIMEType:    mimeType,
		DisplayName: displayName,
	}

	file, err := c.fileClient.Upload(ctx, r, uploadCfg)
	if err != nil {
		return "", "", fmt.Errorf("gemini File API へのアップロードに失敗しました: %w", err)
	}

	// Active 状態になるのを待機
	uri, err := c.waitForFileActive(ctx, file.Name)
	if err != nil {
		// アップロード自体は成功しているため、クリーンアップのためにファイル名を渡す
		c.asyncDelete(file.Name)
		return "", "", fmt.Errorf("ファイル %q が有効状態になるまでの待機中にエラーが発生しました: %w", file.Name, err)
	}

	return uri, file.Name, nil
}

// DeleteFile は指定された名前のファイルを File API から削除します。
func (c *Client) DeleteFile(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	_, err := c.fileClient.Delete(ctx, name, nil)
	if err != nil {
		return fmt.Errorf("ファイル %q の削除に失敗しました: %w", name, err)
	}
	slog.InfoContext(ctx, "File API オブジェクトを削除しました", "name", name)
	return nil
}

// waitForFileActive は指定されたファイルが利用可能になるまでポーリングします。
func (c *Client) waitForFileActive(ctx context.Context, fileName string) (string, error) {
	if uri, done, err := c.checkFileState(ctx, fileName); done || err != nil {
		return uri, err
	}

	ticker := time.NewTicker(c.filePollingInterval)
	defer ticker.Stop()

	timeout := time.NewTimer(c.filePollingTimeout)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("ファイル %q の待機中にコンテキストがキャンセルされました: %w", fileName, ctx.Err())

		case <-timeout.C:
			return "", fmt.Errorf("ファイル %q の処理が制限時間（%v）内に完了しませんでした", fileName, c.filePollingTimeout)

		case <-ticker.C:
			if uri, done, err := c.checkFileState(ctx, fileName); done || err != nil {
				return uri, err
			}
		}
	}
}

func (c *Client) checkFileState(ctx context.Context, fileName string) (uri string, done bool, err error) {
	f, err := c.fileClient.Get(ctx, fileName, &genai.GetFileConfig{})
	if err != nil {
		return "", false, fmt.Errorf("ファイル %q のステータス確認に失敗しました: %w", fileName, err)
	}

	switch f.State {
	case genai.FileStateActive:
		return f.URI, true, nil
	case genai.FileStateFailed:
		return "", true, fmt.Errorf("サーバー側でのファイル処理に失敗しました: %q", fileName)
	case genai.FileStateProcessing:
		slog.DebugContext(ctx, "Gemini File API で処理中...", "name", fileName)
	default:
		slog.WarnContext(ctx, "予期しないファイルステータスを受信しました", "state", f.State, "name", fileName)
	}
	return "", false, nil
}

// asyncDelete はエラー時などの後処理として、バックグラウンドでファイルを削除します。
func (c *Client) asyncDelete(fileName string) {
	go func() {
		// メインの context がキャンセルされていても実行できるよう、新しい context を作成
		ctx, cancel := context.WithTimeout(context.Background(), AsyncCleanupTimeout)
		defer cancel()
		if err := c.DeleteFile(ctx, fileName); err != nil {
			slog.WarnContext(ctx, "バックグラウンドでのファイルクリーンアップに失敗しました", "name", fileName, "error", err)
		}
	}()
}
