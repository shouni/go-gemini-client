package lyria

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/sync/singleflight"
)

// singleflightExecTimeout は、singleflight で共有される生成処理1回あたりの実行タイムアウトです。
// 呼び出し元の context から切り離した実行用 context に適用されます。
const singleflightExecTimeout = 5 * time.Minute

// singleflightKey は namespace と可変長の部品から衝突しにくい singleflight 用キーを作ります。
func singleflightKey(namespace string, parts ...string) string {
	hasher := sha256.New()
	for _, part := range parts {
		hasher.Write([]byte(strconv.Itoa(len(part))))
		hasher.Write([]byte{0})
		hasher.Write([]byte(part))
		hasher.Write([]byte{0})
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
	lengthBuf := make([]byte, 8)
	for _, image := range images {
		if len(image.Data) == 0 {
			continue
		}

		mimeType := image.MIMEType
		hasher.Write([]byte(mimeType))
		hasher.Write([]byte{0})
		binary.LittleEndian.PutUint64(lengthBuf, uint64(len(image.Data)))
		hasher.Write(lengthBuf)
		hasher.Write(image.Data)
		hasher.Write([]byte{0})
	}

	return "images:" + hex.EncodeToString(hasher.Sum(nil))
}

// doSingleflight は同じ key の同時実行をまとめ、呼び出し元のキャンセルも尊重します。
func doSingleflight[T any](ctx context.Context, group *singleflight.Group, key string, fn func(execCtx context.Context) (T, error)) (T, error) {
	execCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), singleflightExecTimeout)
	defer cancel()
	ch := group.DoChan(key, func() (any, error) {
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
		if !ok {
			var zero T
			return zero, fmt.Errorf("singleflight result type mismatch for key %s", key)
		}
		return value, nil
	}
}

// cloneLyricsDraft は LyricsDraft を呼び出し元が安全に変更できるように複製します。
func cloneLyricsDraft(src *LyricsDraft) *LyricsDraft {
	if src == nil {
		return nil
	}

	dst := *src
	dst.Keywords = append([]string(nil), src.Keywords...)
	return &dst
}

// cloneMusicRecipe は MusicRecipe と内部のスライスやポインタを複製します。
func cloneMusicRecipe(src *MusicRecipe) *MusicRecipe {
	if src == nil {
		return nil
	}

	dst := *src
	dst.Instruments = append([]string(nil), src.Instruments...)
	if src.Sections != nil {
		dst.Sections = make([]MusicSection, len(src.Sections))
		copy(dst.Sections, src.Sections)
	}
	dst.Lyrics = cloneLyricsDraft(src.Lyrics)
	if src.Seed != nil {
		v := *src.Seed
		dst.Seed = &v
	}
	return &dst
}

// cloneBytes はバイト列を呼び出し元が安全に変更できるように複製します。
func cloneBytes(src []byte) []byte {
	return append([]byte(nil), src...)
}
