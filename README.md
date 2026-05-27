# ✨ Go Gemini Client

[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/go-gemini-client)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/go-gemini-client)](https://github.com/shouni/go-gemini-client/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/shouni/go-gemini-client)](https://goreportcard.com/report/github.com/shouni/go-gemini-client)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/go-gemini-client.svg)](https://pkg.go.dev/github.com/shouni/go-gemini-client)

## 🎯 概要: Net Armor 統合型ハイブリッド Gemini クライアント

**Go Gemini Client** は、[shouni/netarmor](https://github.com/shouni/netarmor) をリトライ基盤に採用した、**Google Gemini API / Vertex AI** 向けの Go ライブラリです。

ひとつのクライアントで、API Key 方式の **Gemini API (Google AI Studio)** と、Google Cloud 認証を使う **Vertex AI** を切り替えて利用できます。テキスト生成だけでなく、GCS URI や File API を使ったマルチモーダル入力、画像・音声レスポンス、Lyria による音楽生成ワークフローも扱えるように設計されています。

---

## 💎 特徴と設計思想

### 🤖 ハイブリッド・バックエンド・サポート

- **Dual Backend**: `APIKey` 方式と `ProjectID` / `LocationID` 方式の両方に対応。
- **Vertex AI 連携**: Cloud Run などの環境ではサービスアカウントや Application Default Credentials を利用できます。
- **GCS 直接参照**: Vertex AI では `gs://` URI を `genai.Part` として直接プロンプトに含められます。

### 🛡️ 堅牢な AI クライアント (`gemini`)

- **高度なリトライ戦略**: `netarmor` の retry を利用し、一時的なネットワーク障害や API 側の一過性エラーを指数バックオフで再試行します。
- **リトライ不要エラーの判定**: セーフティフィルタによるブロックや空レスポンスなど、再試行しても解決しにくい API レスポンスエラーを識別します。
- **決定論的な制御**: `Seed` により、生成結果の再現性を必要とするワークフローをサポートします。
- **型安全なエラー判定**: 設定不備や入力不備はセンチネルエラーとして公開しており、`errors.Is` で判定できます。

### 📁 高度なリソース管理

- **File API サポート**: ファイルアップロード後、利用可能な `Active` 状態になるまで自動でポーリングします。
- **自動クリーンアップ**: Active 化に失敗した File API オブジェクトはバックグラウンドで削除を試みます。
- **レスポンス抽出**: テキスト、生成画像、生成音声を `gemini.Response` にまとめて返します。

### 🎼 Lyria ワークフロー (`lyria`)

- **作詞から音声生成までの統合**: 歌詞生成、作曲レシピ生成、Lyria 音声生成を `Adapter` で一括実行できます。
- **セクション別生成**: 曲のセクションごとに音声を生成し、WAV として結合できます。
- **重複呼び出し抑制**: singleflight により、同一条件の音声生成リクエストをまとめます。

---

## 🚀 クイックスタート

### インストール

```sh
go get github.com/shouni/go-gemini-client
```

### 1. Gemini API モード (API Key 方式)

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/shouni/go-gemini-client/gemini"
)

func main() {
	ctx := context.Background()

	client, err := gemini.NewClient(ctx, gemini.Config{
		APIKey: "YOUR_GEMINI_API_KEY",
	})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.GenerateContent(ctx, "gemini-2.5-flash", "Goで短い俳句を書いて")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(resp.Text)
}
```

### 2. Vertex AI モード (Cloud Run / GCS 連携)

```go
client, err := gemini.NewClient(ctx, gemini.Config{
	ProjectID:  "your-google-cloud-project-id",
	LocationID: "asia-northeast1",
})
if err != nil {
	return err
}
```

Vertex AI モードでは、Google Cloud 側の認証情報を利用します。Cloud Run などの環境では API Key をアプリケーションに持たせずに運用できます。

---

## 🧩 マルチモーダル生成

`GenerateWithParts` は公式 SDK の `genai.Part` をそのまま受け取ります。テキスト、画像、GCS URI、File API の URI などを組み合わせた入力に対応できます。

```go
parts := []*genai.Part{
	{
		FileData: &genai.FileData{
			URI:      "gs://my-bucket/sample.jpg",
			MIMEType: "image/jpeg",
		},
	},
	{Text: "この画像の内容を日本語で要約してください"},
}

resp, err := client.GenerateWithParts(ctx, "gemini-2.5-flash", parts, gemini.GenerateOptions{
	SystemPrompt: "簡潔に回答してください。",
})
if err != nil {
	return err
}

fmt.Println(resp.Text)
```

---

## 🖼️ 画像・音声レスポンス

`ResponseMIMEType` に `image/*` または `audio/*` を指定すると、レスポンスモダリティが自動設定されます。Inline data は `Response.Images` または `Response.Audios` に格納されます。

```go
seed := int64(1234)

resp, err := client.GenerateWithParts(ctx, "gemini-2.5-flash-image-preview", []*genai.Part{
	{Text: "青い招き猫のステッカー画像を生成して"},
}, gemini.GenerateOptions{
	ResponseMIMEType: "image/png",
	AspectRatio:      "1:1",
	ImageSize:        "1K",
	Seed:             &seed,
})
if err != nil {
	return err
}

if len(resp.Images) > 0 {
	// resp.Images[0] contains image bytes.
}
```

---

## 📤 File API

Gemini API の File API を使う場合は、アップロード後にファイルが `Active` になるまで自動で待機します。

```go
f, err := os.Open("movie.mp4")
if err != nil {
	return err
}
defer f.Close()

uri, name, err := client.UploadFile(ctx, f, "video/mp4", "movie.mp4")
if err != nil {
	return err
}
defer client.DeleteFile(context.Background(), name)

resp, err := client.GenerateWithParts(ctx, "gemini-2.5-flash", []*genai.Part{
	{
		FileData: &genai.FileData{
			URI:      uri,
			MIMEType: "video/mp4",
		},
	},
	{Text: "この動画を要約してください"},
}, gemini.GenerateOptions{})
```

---

## 🎵 Lyria Adapter

`lyria` パッケージは、歌詞生成・作曲レシピ生成・Lyria 音声生成を束ねるファサードです。利用側で `TextPromptGenerator` と `AudioPromptBuilder` を実装し、プロダクト固有のプロンプト設計を差し込めます。

```go
adapter, err := lyria.NewAdapter(
	client,
	promptGenerator,
	lyria.WithGeminiModel("gemini-2.5-flash"),
	lyria.WithLyriaModel("lyria-realtime-exp"),
	lyria.WithAudioPromptBuilder(audioPromptBuilder),
	lyria.WithMaxConcurrency(2),
)
if err != nil {
	return err
}

recipe, wavBytes, err := adapter.Run(ctx, lyria.AIModels{}, &lyria.CollectedContent{
	Prompt: "夜の東京を走るシンセポップ",
})
```

---

## ⚙️ 詳細設定 (`gemini.Config`)

| 設定項目 | 役割 | デフォルト値 |
| --- | --- | --- |
| `APIKey` | Gemini API キー。Google AI Studio / Gemini API で利用します。 | - |
| `ProjectID` | Google Cloud プロジェクト ID。Vertex AI で利用します。 | - |
| `LocationID` | Vertex AI のリージョン。例: `asia-northeast1`, `us-central1` | - |
| `MaxRetries` | 最大リトライ回数 | `1` |
| `InitialDelay` | リトライ開始時の待機時間 | `30s` |
| `MaxDelay` | リトライ待機時間の上限 | `120s` |
| `FilePollingInterval` | File API の状態確認間隔 | `2s` |
| `FilePollingTimeout` | File API の状態確認タイムアウト | `60s` |

`APIKey` と `ProjectID` / `LocationID` は排他的です。Vertex AI を使う場合は `ProjectID` と `LocationID` の両方を指定してください。

---

## 🧪 生成オプション (`gemini.GenerateOptions`)

| 設定項目 | 役割 |
| --- | --- |
| `SystemPrompt` | System instruction を指定します。 |
| `AspectRatio` | 画像生成時のアスペクト比を指定します。 |
| `ImageSize` | 画像生成時のサイズを指定します。 |
| `Seed` | 再現性のためのシード値。`int32` の範囲内である必要があります。 |
| `PersonGeneration` | Vertex AI 画像生成での人物生成ポリシーを指定します。 |
| `SafetySettings` | SDK の SafetySettings を指定します。 |
| `ResponseMIMEType` | `image/png` や `audio/wav` など、期待するレスポンス MIME type を指定します。 |

---

## 📜 エラーハンドリング

本ライブラリでは、以下のセンチネルエラーをエクスポートしています。`errors.Is` を使って判定できます。

- `ErrConfigRequired`: `APIKey` または `ProjectID` / `LocationID` のいずれも設定されていない場合。
- `ErrExclusiveConfig`: `APIKey` と `ProjectID` / `LocationID` が同時に設定されている場合。
- `ErrIncompleteVertexConfig`: `ProjectID` または `LocationID` の片方だけが設定されている場合。
- `ErrEmptyPrompt`: プロンプトが空の場合。
- `ErrEmptyModelName`: モデル名が空の場合。
- `ErrEmptyParts`: 生成パーツが空の場合。
- `ErrInvalidPart`: 生成パーツに nil が含まれている場合。
- `ErrInvalidSeed`: `Seed` が `int32` の範囲外の場合。

---

## 📂 パッケージ構成

| パッケージ | 役割 |
| --- | --- |
| `github.com/shouni/go-gemini-client/gemini` | Gemini / Vertex AI クライアント、リトライ、File API、レスポンス抽出。 |
| `github.com/shouni/go-gemini-client/lyria` | 歌詞生成、作曲レシピ生成、Lyria 音声生成の統合アダプタ。 |

---

## 🤝 依存関係 (Dependencies)

- [google.golang.org/genai](https://pkg.go.dev/google.golang.org/genai) - Google Gemini 公式 SDK
- [shouni/netarmor](https://github.com/shouni/netarmor) - ネットワークセキュリティ & リトライ戦略
- [shouni/audio](https://github.com/shouni/audio) - WAV 結合・音声処理ユーティリティ

---

## 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。
