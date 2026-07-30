package veo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shouni/go-gemini-client/gemini"
)

// 入力・結果に関するセンチネルエラーです。呼び出し側は errors.Is で判定し、
// 通信エラーとは異なる制御（プロンプトの見直しなど）を選べます。
var (
	// ErrGeneratorRequired は、New に nil の生成クライアントが渡された場合に返されます。
	ErrGeneratorRequired = errors.New("veo: video generator is required")

	// ErrMissingOperationName は、完了待ちに必要なオペレーション名が無い場合に
	// 返されます。未完了のオペレーションは必ず名前を持つため、通常は起こりません。
	ErrMissingOperationName = errors.New("veo: operation has no name to poll")

	// ErrNoVideoGenerated は、オペレーションは成功で完了したのに動画が1本も
	// 返らなかった場合に返されます。安全性ポリシーによる除外が典型例で、
	// その場合は理由がエラーメッセージに含まれます。再試行では解決しません。
	ErrNoVideoGenerated = errors.New("veo: no video was generated")

	// ErrPollFailed は、生成状況の確認が続けて失敗し、完了を待てなくなった場合に
	// 返されます。最後に発生した原因が Unwrap で辿れます。
	ErrPollFailed = errors.New("veo: polling for completion failed")
)

// Client は動画生成の投函から完了待ちまでを扱うクライアントです。
type Client struct {
	generator     gemini.VideoGenerator
	pollInterval  time.Duration
	pollTimeout   time.Duration
	maxPollErrors int
}

// New は、動画生成クライアントを注入して Client を初期化します。
//
// generator には *gemini.Client をそのまま渡せます。
//
//	gc, err := gemini.NewClient(ctx, cfg)
//	vc, err := veo.New(gc, veo.WithPollInterval(10*time.Second))
func New(generator gemini.VideoGenerator, opts ...Option) (*Client, error) {
	if generator == nil {
		return nil, ErrGeneratorRequired
	}
	c := &Client{
		generator:     generator,
		pollInterval:  DefaultPollInterval,
		pollTimeout:   DefaultPollTimeout,
		maxPollErrors: DefaultMaxPollErrors,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Generate は動画生成を開始し、完了するまで待って結果を返します。
//
// 生成そのものが失敗した場合（安全性ポリシーによるブロックなど）は
// gemini.ErrVideoGenerationFailed を含むエラーになります。完了を待てなかった場合は
// ポーリングの打ち切り理由（タイムアウトまたは ErrPollFailed）を返します。
func (c *Client) Generate(ctx context.Context, modelName string, req Request) (*Result, error) {
	op, err := c.generator.StartVideo(ctx, modelName, req)
	if err != nil {
		return nil, err
	}
	if op == nil {
		return nil, fmt.Errorf("veo: %w", gemini.ErrEmptyResponse)
	}
	slog.InfoContext(ctx, "動画生成オペレーションを開始しました", "operation", op.Name, "model", modelName)

	if op.Done {
		return resultFrom(op)
	}
	if strings.TrimSpace(op.Name) == "" {
		return nil, ErrMissingOperationName
	}
	return c.Wait(ctx, op.Name)
}

// Wait は、開始済みの動画生成オペレーションが完了するまでポーリングして結果を返します。
//
// Generate から分けているのは、投函と完了待ちを別のプロセス・別の実行で行える
// ようにするためです（実行時間に上限のあるジョブ基盤で、投函だけ済ませて一旦戻り、
// 次の実行で名前を渡して待ちを再開する、といった使い方ができます）。
//
// 1回ごとの問い合わせにはリトライを掛けません。一時的な失敗はこのループが
// maxPollErrors 回まで受け流し、それを超えた時点で ErrPollFailed として打ち切ります
// （gemini.PollVideo のコメント参照）。
func (c *Client) Wait(ctx context.Context, operationName string) (*Result, error) {
	if strings.TrimSpace(operationName) == "" {
		return nil, ErrMissingOperationName
	}

	waitCtx, cancel := context.WithTimeout(ctx, c.pollTimeout)
	defer cancel()

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	consecutiveErrors := 0
	for {
		select {
		case <-waitCtx.Done():
			return nil, c.waitDeadlineError(ctx, operationName, waitCtx.Err())
		case <-ticker.C:
		}

		op, err := c.generator.PollVideo(waitCtx, operationName)
		if err != nil {
			// 待ち時間の上限に達したことによる失敗は「一時的な失敗」ではないので
			// 回数に数えず、次のループで打ち切り理由をそのまま返す。
			if waitCtx.Err() != nil {
				continue
			}
			consecutiveErrors++
			if consecutiveErrors >= c.maxPollErrors {
				return nil, fmt.Errorf("%w: オペレーション %q の確認が %d 回連続で失敗しました: %w",
					ErrPollFailed, operationName, consecutiveErrors, err)
			}
			slog.WarnContext(ctx, "動画生成の状況確認に失敗しました。再確認します",
				"operation", operationName, "consecutive_errors", consecutiveErrors, "error", err)
			continue
		}

		consecutiveErrors = 0
		if op.Done {
			return resultFrom(op)
		}
	}
}

// waitDeadlineError は、完了待ちを打ち切った理由をエラーに整えます。
// 呼び出し側の context が終了した場合と、このクライアントのタイムアウトに達した
// 場合とでは対処が異なる（前者は上位のキャンセル、後者は生成が長すぎる）ため、
// 区別できるようにしています。
func (c *Client) waitDeadlineError(ctx context.Context, operationName string, waitErr error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("オペレーション %q の完了待ちが中断されました: %w", operationName, ctx.Err())
	}
	return fmt.Errorf("オペレーション %q が制限時間（%v）内に完了しませんでした: %w",
		operationName, c.pollTimeout, waitErr)
}

// resultFrom は、完了したオペレーションを結果へ変換します。
func resultFrom(op *gemini.VideoOperation) (*Result, error) {
	if op.Failure != nil {
		return nil, fmt.Errorf("オペレーション %q: %w", op.Name, op.Failure)
	}
	if len(op.Videos) == 0 {
		return nil, noVideoError(op)
	}
	return &Result{
		OperationName:   op.Name,
		Videos:          op.Videos,
		FilteredCount:   op.FilteredCount,
		FilteredReasons: op.FilteredReasons,
	}, nil
}

// noVideoError は、成功したのに動画が返らなかった場合のエラーを組み立てます。
// 安全性ポリシーによる除外がこの経路の大半を占めるため、理由が分かっている
// ときはメッセージに含めます（プロンプトを直すのに必要な情報です）。
func noVideoError(op *gemini.VideoOperation) error {
	if op.FilteredCount > 0 || len(op.FilteredReasons) > 0 {
		return fmt.Errorf("%w: 安全性ポリシーにより除外されました（件数=%d, 理由=%s）",
			ErrNoVideoGenerated, op.FilteredCount, strings.Join(op.FilteredReasons, "; "))
	}
	return fmt.Errorf("%w: オペレーション %q", ErrNoVideoGenerated, op.Name)
}
