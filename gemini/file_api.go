package gemini

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"google.golang.org/genai"
)

// fileStateMaxPollErrors は、ファイルの Active 化待ちでステータス確認の連続失敗を
// 許容する回数です。1回の一時的なエラーで待機全体（既定 60 秒の予算）を放棄しない
// ための値で、恒久的な障害ではタイムアウトまで待たずに打ち切ります。
const fileStateMaxPollErrors = 5

// UploadedFile はアップロード済みファイルの参照情報です。
//
// URI と Name はどちらも文字列ですが用途が異なります。URI は生成リクエストの
// genai.FileData に渡す値、Name は DeleteFile に渡す識別子です。
// 取り違えを防ぐため構造体で返しています。
type UploadedFile struct {
	// URI は生成リクエストから参照するための URI です。
	URI string
	// Name は File API 上の識別子です。DeleteFile に渡します。
	Name string
}

// UploadFile はデータをアップロードし、そのファイルが Active 状態になるまで待機します。
// アップロード処理自体が成功した場合、たとえその後の Active 化処理でエラーが発生しても
// サーバー側にリソースが残る可能性があるため、バックグラウンドでの削除を試みます。
//
// アップロードは Config のリトライ設定に従って再送されます。再送に備えて r は
// 最初に全量をメモリへ読み込みます（この経路に来るのは画像・音声などの添付で、
// ストリーミングが必要なサイズのデータは想定していません）。
//
// バックグラウンド削除は投げっぱなしで、完了を待つ手段はありません。
// 確実に削除したい場合は呼び出し側で DeleteFile を呼んでください。
func (c *Client) UploadFile(ctx context.Context, r io.Reader, mimeType, displayName string) (UploadedFile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return UploadedFile{}, fmt.Errorf("アップロードデータの読み込みに失敗しました: %w", err)
	}

	uploadCfg := &genai.UploadFileConfig{
		MIMEType:    mimeType,
		DisplayName: displayName,
	}

	file, err := runWithRetry(ctx, c.retryOpts, "File API Upload", func() (*genai.File, error) {
		return c.fileClient.Upload(ctx, bytes.NewReader(data), uploadCfg)
	})
	if err != nil {
		return UploadedFile{}, fmt.Errorf("gemini File API へのアップロードに失敗しました: %w", err)
	}

	// Active 状態になるのを待機
	uri, err := c.waitForFileActive(ctx, file.Name)
	if err != nil {
		// アップロード自体は成功しているため、クリーンアップのためにファイル名を渡す
		c.asyncDelete(ctx, file.Name)
		return UploadedFile{}, fmt.Errorf("ファイル %q が有効状態になるまでの待機中にエラーが発生しました: %w", file.Name, err)
	}

	return UploadedFile{URI: uri, Name: file.Name}, nil
}

// DeleteFile は指定された名前のファイルを File API から削除します。
//
// 削除は Config のリトライ設定に従って再送されます。ファイルが既に存在しない場合
// （前回の削除が実は成功していた場合など）は目的を達成しているため成功扱いです。
func (c *Client) DeleteFile(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	_, err := runWithRetry(ctx, c.retryOpts, "File API Delete", func() (*genai.DeleteFileResponse, error) {
		resp, err := c.fileClient.Delete(ctx, name, nil)
		if err != nil && isNotFoundAPIError(err) {
			return nil, nil
		}
		return resp, err
	})
	if err != nil {
		return fmt.Errorf("ファイル %q の削除に失敗しました: %w", name, err)
	}
	c.log().InfoContext(ctx, "File API オブジェクトを削除しました", "name", name)
	return nil
}

// isNotFoundAPIError は、genai の API エラーが 404 (Not Found) かを判定します。
func isNotFoundAPIError(err error) bool {
	apiErr, ok := errors.AsType[genai.APIError](err)
	return ok && apiErr.Code == http.StatusNotFound
}

// waitForFileActive は指定されたファイルが利用可能になるまでポーリングします。
//
// ステータス確認1回ごとにはリトライを掛けず、一時的な失敗はこのループが
// fileStateMaxPollErrors 回まで受け流します。確認の内部でバックオフを効かせると
// ポーリング間隔とタイムアウトの意味が失われるためです（veo.Client と同じ方針）。
func (c *Client) waitForFileActive(ctx context.Context, fileName string) (string, error) {
	consecutiveErrors := 0

	// poll は1回のステータス確認を行い、待機を終えるべきかを返します。
	poll := func() (uri string, finished bool, err error) {
		uri, done, err := c.checkFileState(ctx, fileName)
		if done {
			return uri, true, err
		}
		if err == nil {
			consecutiveErrors = 0
			return "", false, nil
		}
		// 呼び出し元のキャンセルは「一時的な失敗」ではないため即座に返す。
		if ctx.Err() != nil {
			return "", true, fmt.Errorf("ファイル %q の待機中にコンテキストがキャンセルされました: %w", fileName, ctx.Err())
		}
		consecutiveErrors++
		if consecutiveErrors >= fileStateMaxPollErrors {
			return "", true, fmt.Errorf("ファイル %q のステータス確認が %d 回連続で失敗しました: %w", fileName, consecutiveErrors, err)
		}
		c.log().WarnContext(ctx, "ファイルステータスの確認に失敗しました。再確認します",
			"name", fileName, "consecutive_errors", consecutiveErrors, "error", err)
		return "", false, nil
	}

	if uri, finished, err := poll(); finished {
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
			if uri, finished, err := poll(); finished {
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
		c.log().DebugContext(ctx, "Gemini File API で処理中...", "name", fileName)
	default:
		c.log().WarnContext(ctx, "予期しないファイルステータスを受信しました", "state", f.State, "name", fileName)
	}
	return "", false, nil
}

// asyncDelete はエラー時などの後処理として、バックグラウンドでファイルを削除します。
func (c *Client) asyncDelete(ctx context.Context, fileName string) {
	go func() {
		// メインの context がキャンセルされていても実行できるようキャンセルだけを
		// 切り離す。context.Background() では trace ID などの値まで消えてしまい、
		// 直後の警告ログがどのリクエスト由来か辿れなくなる。
		timeout := c.asyncCleanupTimeout
		if timeout <= 0 {
			timeout = AsyncCleanupTimeout
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()
		if err := c.DeleteFile(cleanupCtx, fileName); err != nil {
			c.log().WarnContext(cleanupCtx, "バックグラウンドでのファイルクリーンアップに失敗しました", "name", fileName, "error", err)
		}
	}()
}
