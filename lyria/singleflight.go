package lyria

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"strconv"
	"time"

	"golang.org/x/sync/singleflight"
)

// defaultSingleflightExecTimeout は、singleflight で共有される生成処理1回あたりの
// 実行タイムアウトの既定値です。呼び出し元の context から切り離した実行用 context に
// 適用されます。WithExecTimeout で変更できます。
const defaultSingleflightExecTimeout = 5 * time.Minute

// writeHashPart は長さプレフィックス付きでハッシュへ部品を書き込みます。
// 長さを先に書くのは "ab"+"c" と "a"+"bc" のような連結の衝突を防ぐためで、
// キーを組み立てる関数はすべてこの1つの枠組みを共有します。
func writeHashPart(h hash.Hash, part []byte) {
	var lengthBuf [8]byte
	binary.LittleEndian.PutUint64(lengthBuf[:], uint64(len(part)))
	h.Write(lengthBuf[:])
	h.Write(part)
}

// singleflightKey は namespace と可変長の部品から衝突しにくい singleflight 用キーを作ります。
func singleflightKey(namespace string, parts ...string) string {
	hasher := sha256.New()
	for _, part := range parts {
		writeHashPart(hasher, []byte(part))
	}

	return namespace + ":" + hex.EncodeToString(hasher.Sum(nil))
}

// singleflightSeedKey は nil と実値を区別できる seed 用キー部品を作ります。
func singleflightSeedKey(seed *int64) string {
	if seed == nil {
		return "seed:nil"
	}
	return "seed:" + strconv.FormatInt(*seed, 10)
}

// calculateImagesHash は画像ペイロードの内容から singleflight 用のキー部品を作ります。
func calculateImagesHash(images []ImagePayload) string {
	hasher := sha256.New()
	for _, image := range images {
		if len(image.Data) == 0 {
			continue
		}

		writeHashPart(hasher, []byte(image.MIMEType))
		writeHashPart(hasher, image.Data)
	}

	return "images:" + hex.EncodeToString(hasher.Sum(nil))
}

// doSingleflight は同じ key の同時実行をまとめ、呼び出し元のキャンセルも尊重します。
// 実行用 context は共有実行のクロージャ内で呼び出し元から切り離して（WithoutCancel）
// 生成します。呼び出し元側で生成すると、リーダー（最初の呼び出し元）がキャンセルして
// 早期リターンした際に defer cancel() が共有実行を打ち切り、相乗りしている他の
// 呼び出し元が巻き添えになるためです。実行の打ち切りは execTimeout でのみ行われます。
func doSingleflight[T any](ctx context.Context, group *singleflight.Group, key string, execTimeout time.Duration, fn func(execCtx context.Context) (T, error)) (T, error) {
	if execTimeout <= 0 {
		execTimeout = defaultSingleflightExecTimeout
	}
	ch := group.DoChan(key, func() (any, error) {
		execCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), execTimeout)
		defer cancel()
		return fn(execCtx)
	})

	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			var zero T
			return zero, result.Err
		}

		value, ok := result.Val.(T)
		// fn の静的な型付けにより result.Val は常に T なので、この分岐は実際には到達しない。
		// 将来のリファクタリングに対する防御として意図的に残している。
		if !ok {
			var zero T
			return zero, fmt.Errorf("singleflight result type mismatch for key %s", key)
		}
		return value, nil
	}
}

// cloneBytes はバイト列を呼び出し元が安全に変更できるように複製します。
func cloneBytes(src []byte) []byte {
	return append([]byte(nil), src...)
}
