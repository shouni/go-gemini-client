package lyria

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"github.com/shouni/go-gemini-client/callguard"
)

// calculateImagesHash は画像ペイロードの内容から singleflight 用のキー部品を作ります。
//
// 画像はバイト列のまま長さプレフィックス付きでハッシュへ流します。callguard.Key に
// 渡すために文字列へ変換すると、数 MB の複製がキーを作るたびに走るためです。
// 枠組み（長さプレフィックス）は callguard.WriteHashPart と共有しています。
func calculateImagesHash(images []ImagePayload) string {
	hasher := sha256.New()
	for _, image := range images {
		if len(image.Data) == 0 {
			continue
		}

		callguard.WriteHashPart(hasher, []byte(image.MIMEType))
		callguard.WriteHashPart(hasher, image.Data)
	}

	return "images:" + hex.EncodeToString(hasher.Sum(nil))
}

// cloneBytes はバイト列を呼び出し元が安全に変更できるように複製します。
func cloneBytes(src []byte) []byte {
	return slices.Clone(src)
}
