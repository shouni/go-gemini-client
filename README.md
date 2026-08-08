# ✨ Go Gemini Client

[![CI](https://github.com/shouni/go-gemini-client/actions/workflows/ci.yml/badge.svg)](https://github.com/shouni/go-gemini-client/actions/workflows/ci.yml)
[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/go-gemini-client)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/go-gemini-client)](https://github.com/shouni/go-gemini-client/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/go-gemini-client.svg)](https://pkg.go.dev/github.com/shouni/go-gemini-client)
[![Status](https://img.shields.io/badge/Status-Completed-brightgreen)](#)

## 🎯 概要: Net Armor 統合型ハイブリッド Gemini クライアント

**Go Gemini Client** は、[shouni/netarmor](https://github.com/shouni/netarmor) をリトライ基盤に採用した、**Google Gemini API / Vertex AI** 向けの Go ライブラリです。

ひとつのクライアントで、API Key 方式の **Gemini API (Google AI Studio)** と、Google Cloud 認証を使う **Vertex AI** を切り替えて利用できます。テキスト生成だけでなく、GCS URI や File API を使ったマルチモーダル入力、画像・音声レスポンス、Lyria による音楽生成、Veo による動画生成も扱えるように設計されています。

---

## 💎 特徴と設計思想

### 🤖 ハイブリッド・バックエンド・サポート

- **Dual Backend**: `APIKey` 方式と `ProjectID` / `LocationID` 方式の両方に対応。
- **Vertex AI 連携**: Cloud Run などの環境ではサービスアカウントや Application Default Credentials を利用できます。
- **GCS 直接参照**: Vertex AI では `gs://` URI を `gemini.Attachment{URI: ...}` として直接プロンプトに含められます。

### 🛡️ 堅牢な AI クライアント (`gemini`)

- **高度なリトライ戦略**: `netarmor` の retry を利用し、一時的なネットワーク障害や API 側の一過性エラーを指数バックオフで再試行します。
- **リトライ不要エラーの判定**: セーフティフィルタによるブロックや空レスポンスなど、再試行しても解決しにくい API レスポンスエラーを識別します。
- **決定論的な制御**: `Seed` により、生成結果の再現性を必要とするワークフローをサポートします。
- **型安全なエラー判定**: 設定不備や入力不備はセンチネルエラーとして公開しており、`errors.Is` で判定できます。
- **SDK 型を漏らさない入口**: `GenerateWithAttachments` と `gemini.Attachment` を使えば、マルチモーダル生成・構造化出力・安全設定・思考量のいずれも genai SDK を import せずに書けます。モックも 1 メソッドで済みます。
- **ストリーミング生成**: `GenerateContentStream` / `GenerateWithAttachmentsStream` / `GenerateWithPartsStream` で `iter.Seq2` によるチャンク単位のレスポンスを受け取れます。
- **トークン数の事前計測**: `CountTokens` / `CountTokensWithAttachments` / `CountTokensWithParts` で、実際に生成せずにプロンプトのトークン数を見積もれます。

### 📁 高度なリソース管理

- **File API サポート**: ファイルアップロード後、利用可能な `Active` 状態になるまで自動でポーリングします。
- **自動クリーンアップ**: Active 化に失敗した File API オブジェクトはバックグラウンドで削除を試みます。
- **レスポンス抽出**: テキスト、生成画像、生成音声、MIME type 付きの添付 (`Attachments`)、トークン使用量 (`Usage`) を `gemini.Response` にまとめて返します。

### 🎬 Veo 動画生成 (`veo`)

- **長時間実行オペレーションの完走**: 投函から完了までのポーリング、タイムアウト、一時的な失敗の許容をまとめて扱います。投函 (`Submit`) と完了待ち (`Wait`) は別の実行に分けられます。
- **入力の事前検証**: Veo が併用できない入力（video と image など）を送信前に弾きます。
- **genai 非依存**: `gemini.VideoGenerator` の 2 メソッドを注入するだけなので、テストは SDK も認証も不要です。

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

	resp, err := client.GenerateContent(ctx, "gemini-3.6-flash", "Goで短い俳句を書いて")
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

`GenerateWithAttachments` は、テキストと添付（画像・音声・PDF など）を genai SDK の型を使わずに渡せる入口です。`gemini.Attachment` はバイト列と URI 参照のどちらも表現できます。

```go
resp, err := client.GenerateWithAttachments(ctx, "gemini-3.6-flash",
	"この画像の内容を日本語で要約してください",
	[]gemini.Attachment{
		// Vertex AI では gs:// を直接参照でき、File API の files/... も同じ形で渡せます。
		{URI: "gs://my-bucket/sample.jpg", MIMEType: "image/jpeg"},
		// バイト列を送る場合は Data を使います（URI とは排他）。
		// {MIMEType: "image/png", Data: pngBytes},
	},
	gemini.GenerateOptions{SystemPrompt: "簡潔に回答してください。"})
if err != nil {
	return err
}

fmt.Println(resp.Text)
```

添付の扱いは次のとおりです。

- `Data` と `URI` は排他で、両方設定すると `ErrInvalidAttachment` になります
- `Data` を送る場合 `MIMEType` は必須、`URI` を参照する場合は任意（省略するとサーバー側の判定に委ねます）
- どちらも空の要素は読み飛ばされます。参照画像を「あれば渡す」形で組み立てる呼び出し側が、空要素の除去を毎回書かずに済みます
- プロンプトが空でも添付があれば送信できます（音声だけを渡して解析させる用途）
- 添付が無いテキスト生成でも使えます。`attachments` に `nil` を渡せば「プロンプト + `GenerateOptions`」の入口になり、オプション無しの `GenerateContent` を補完します

プロンプトは添付より前に置かれます。genai SDK の `genai.Part` を直接受け取る公開 API は意図的にありません。SDK の型を公開面へ漏らすと、利用側が genai を import する理由が復活してしまうためです。

---

## 🖼️ 画像・音声レスポンス

`ResponseMIMEType` に `image/*` または `audio/*` を指定すると、レスポンスモダリティが自動設定されます。Inline data は `Response.Images` または `Response.Audios` に格納されます。

MIME type も必要な場合は `Response.Attachments` を使います。`Images` / `Audios` はバイト列だけなので、保存時の拡張子や Content-Type を決める必要がある場合は、返却順のまま `gemini.Attachment`（MIME type + バイト列）で受け取れる `Attachments` を使ってください。

```go
seed := int64(1234)

resp, err := client.GenerateWithAttachments(ctx, "gemini-3.1-flash-image",
	"青い招き猫のステッカー画像を生成して", nil,
	gemini.GenerateOptions{
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

for _, attachment := range resp.Attachments {
	// attachment.MIMEType で保存時の拡張子や Content-Type を決められます。
	_ = attachment
}
```

### `gemini.Response` の中身

| フィールド | 内容 |
| --- | --- |
| `Text` | 本文。思考パートを除く全テキストパートの連結です。 |
| `Images` / `Audios` | インラインデータのバイト列を MIME type で振り分けたものです。 |
| `Attachments` | インラインデータを MIME type 付きで返却順に保持します（`Images` / `Audios` の上位集合）。 |
| `Thoughts` | 思考サマリ。`IncludeThoughts` が true でモデルが返した場合のみ設定され、`Text` には含まれません。 |
| `Usage` | トークン使用量（`*gemini.TokenUsage`）。`PromptTokenCount` / `CandidatesTokenCount` / `TotalTokenCount` に加え、課金対象の `ThoughtsTokenCount` を持ちます。 |

---

## 📤 File API

Gemini API の File API を使う場合は、アップロード後にファイルが `Active` になるまで自動で待機します。

`UploadFile` は `gemini.UploadedFile{URI, Name}` を返します。`URI` は生成リクエストから参照する値、`Name` は `DeleteFile` に渡す識別子で、用途が違うため構造体で返しています。

```go
f, err := os.Open("movie.mp4")
if err != nil {
	return err
}
defer f.Close()

uploaded, err := client.UploadFile(ctx, f, "video/mp4", "movie.mp4")
if err != nil {
	return err
}
defer client.DeleteFile(context.Background(), uploaded.Name)

resp, err := client.GenerateWithAttachments(ctx, "gemini-3.6-flash",
	"この動画を要約してください",
	[]gemini.Attachment{{URI: uploaded.URI, MIMEType: "video/mp4"}},
	gemini.GenerateOptions{})
```

File API の呼び出しにも `Config` のリトライ設定が効きます。

- **Upload** は再送に備えて入力を最初に全量メモリへ読み込みます（画像・音声などの添付が対象で、ストリーミングが必要なサイズは想定していません）
- **Delete** は対象が既に存在しない場合（前回の削除が実は成功していた場合など）を成功として扱います
- **Active 化待ちのステータス確認**にはリトライを掛けず、一時的な失敗をループ側で 5 回まで受け流します（ポーリングの内部でバックオフを効かせると間隔とタイムアウトの意味が失われるためです）
- 失敗時のバックグラウンド削除の上限時間は `Config.AsyncCleanupTimeout`（既定 15 秒）で調整できます

---

## 🎬 Veo 動画生成 (`veo`)

- **長時間実行オペレーションの完走**: 投函から完了までのポーリング、タイムアウト、一時的な失敗の許容をまとめて扱います。投函 (`Submit`) と完了待ち (`Wait`) は別の実行に分けられます。
- **入力の事前検証**: Veo が併用できない入力（video と image など）を送信前に弾きます。
- **genai 非依存**: `gemini.VideoGenerator` の 2 メソッドを注入するだけなので、テストは SDK も認証も不要です。

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

	resp, err := client.GenerateContent(ctx, "gemini-3.6-flash", "Goで短い俳句を書いて")
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

`GenerateWithAttachments` は、テキストと添付（画像・音声・PDF など）を genai SDK の型を使わずに渡せる入口です。`gemini.Attachment` はバイト列と URI 参照のどちらも表現できます。

```go
resp, err := client.GenerateWithAttachments(ctx, "gemini-3.6-flash",
	"この画像の内容を日本語で要約してください",
	[]gemini.Attachment{
		// Vertex AI では gs:// を直接参照でき、File API の files/... も同じ形で渡せます。
		{URI: "gs://my-bucket/sample.jpg", MIMEType: "image/jpeg"},
		// バイト列を送る場合は Data を使います（URI とは排他）。
		// {MIMEType: "image/png", Data: pngBytes},
	},
	gemini.GenerateOptions{SystemPrompt: "簡潔に回答してください。"})
if err != nil {
	return err
}

fmt.Println(resp.Text)
```

添付の扱いは次のとおりです。

- `Data` と `URI` は排他で、両方設定すると `ErrInvalidAttachment` になります
- `Data` を送る場合 `MIMEType` は必須、`URI` を参照する場合は任意（省略するとサーバー側の判定に委ねます）
- どちらも空の要素は読み飛ばされます。参照画像を「あれば渡す」形で組み立てる呼び出し側が、空要素の除去を毎回書かずに済みます
- プロンプトが空でも添付があれば送信できます（音声だけを渡して解析させる用途）
- 添付が無いテキスト生成でも使えます。`attachments` に `nil` を渡せば「プロンプト + `GenerateOptions`」の入口になり、オプション無しの `GenerateContent` を補完します

プロンプトは添付より前に置かれます。genai SDK の `genai.Part` を直接受け取る公開 API は意図的にありません。SDK の型を公開面へ漏らすと、利用側が genai を import する理由が復活してしまうためです。

---

## 🖼️ 画像・音声レスポンス

`ResponseMIMEType` に `image/*` または `audio/*` を指定すると、レスポンスモダリティが自動設定されます。Inline data は `Response.Images` または `Response.Audios` に格納されます。

MIME type も必要な場合は `Response.Attachments` を使います。`Images` / `Audios` はバイト列だけなので、保存時の拡張子や Content-Type を決める必要がある場合は、返却順のまま `gemini.Attachment`（MIME type + バイト列）で受け取れる `Attachments` を使ってください。

```go
seed := int64(1234)

resp, err := client.GenerateWithAttachments(ctx, "gemini-3.1-flash-image",
	"青い招き猫のステッカー画像を生成して", nil,
	gemini.GenerateOptions{
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

for _, attachment := range resp.Attachments {
	// attachment.MIMEType で保存時の拡張子や Content-Type を決められます。
	_ = attachment
}
```

### `gemini.Response` の中身

| フィールド | 内容 |
| --- | --- |
| `Text` | 本文。思考パートを除く全テキストパートの連結です。 |
| `Images` / `Audios` | インラインデータのバイト列を MIME type で振り分けたものです。 |
| `Attachments` | インラインデータを MIME type 付きで返却順に保持します（`Images` / `Audios` の上位集合）。 |
| `Thoughts` | 思考サマリ。`IncludeThoughts` が true でモデルが返した場合のみ設定され、`Text` には含まれません。 |
| `Usage` | トークン使用量（`*gemini.TokenUsage`）。`PromptTokenCount` / `CandidatesTokenCount` / `TotalTokenCount` に加え、課金対象の `ThoughtsTokenCount` を持ちます。 |

---

## 📤 File API

Gemini API の File API を使う場合は、アップロード後にファイルが `Active` になるまで自動で待機します。

`UploadFile` は `gemini.UploadedFile{URI, Name}` を返します。`URI` は生成リクエストから参照する値、`Name` は `DeleteFile` に渡す識別子で、用途が違うため構造体で返しています。

```go
f, err := os.Open("movie.mp4")
if err != nil {
	return err
}
defer f.Close()

uploaded, err := client.UploadFile(ctx, f, "video/mp4", "movie.mp4")
if err != nil {
	return err
}
defer client.DeleteFile(context.Background(), uploaded.Name)

resp, err := client.GenerateWithAttachments(ctx, "gemini-3.6-flash",
	"この動画を要約してください",
	[]gemini.Attachment{{URI: uploaded.URI, MIMEType: "video/mp4"}},
	gemini.GenerateOptions{})
```

File API の呼び出しにも `Config` のリトライ設定が効きます。

- **Upload** は再送に備えて入力を最初に全量メモリへ読み込みます（画像・音声などの添付が対象で、ストリーミングが必要なサイズは想定していません）
- **Delete** は対象が既に存在しない場合（前回の削除が実は成功していた場合など）を成功として扱います
- **Active 化待ちのステータス確認**にはリトライを掛けず、一時的な失敗をループ側で 5 回まで受け流します（ポーリングの内部でバックオフを効かせると間隔とタイムアウトの意味が失われるためです）
- 失敗時のバックグラウンド削除の上限時間は `Config.AsyncCleanupTimeout`（既定 15 秒）で調整できます

---

## 📶 ストリーミング生成

`GenerateContentStream` / `GenerateWithAttachmentsStream` / `GenerateWithPartsStream` は、genai SDK の `iter.Seq2` をそのまま `gemini.Response` のストリームに変換して返します。チャンク単位でエラーが発生した場合は、そのチャンクの `error` 戻り値として伝播します（ストリーム開始後のリトライは行いません）。

```go
seq, err := client.GenerateContentStream(ctx, "gemini-3.6-flash", "Goについて3行で説明して")
if err != nil {
	return err
}

for resp, err := range seq {
	if err != nil {
		return err
	}
	fmt.Print(resp.Text)
}
```

添付付きの生成をストリーミングで受け取る場合は `GenerateWithAttachmentsStream` を使います。入力の検証は `GenerateWithAttachments` と同じで、ストリーム開始前に失敗します。

```go
seq, err := client.GenerateWithAttachmentsStream(ctx, "gemini-3.6-flash",
	"この画像の内容を実況してください",
	[]gemini.Attachment{{URI: "gs://my-bucket/sample.jpg", MIMEType: "image/jpeg"}},
	gemini.GenerateOptions{})
```

---

## 🔢 トークン数の計測

`CountTokens` / `CountTokensWithAttachments` / `CountTokensWithParts` は、実際に生成を行わずにプロンプトのトークン数だけを計測します。事前のコスト見積もりやコンテキスト長の検証に使えます。

```go
total, err := client.CountTokens(ctx, "gemini-3.6-flash", "Goについて3行で説明して")
if err != nil {
	return err
}
fmt.Println("推定トークン数:", total)
```

画像や音声はテキストよりトークン数が読みにくく、送る前に測りたいのはむしろこちらです。添付を含めた見積もりは `CountTokensWithAttachments` で、genai SDK を import せずに行えます。

```go
total, err := client.CountTokensWithAttachments(ctx, "gemini-3.6-flash", "この動画を要約して",
	[]gemini.Attachment{{URI: "gs://my-bucket/clip.mp4", MIMEType: "video/mp4"}})
```

生成レスポンス自体のトークン使用量は `Response.Usage` から参照できます（`PromptTokenCount` / `CandidatesTokenCount` / `TotalTokenCount`、および思考に消費された `ThoughtsTokenCount`）。思考機能を使う場合、`ThoughtsTokenCount` も課金対象です。

---

## 🎬 Veo 動画生成 (`veo`)

`veo` パッケージは Veo による動画生成を扱います。動画生成は長時間実行オペレーションで、投函してから完了までポーリングし続ける必要があります。**「1往復ずつ」を `gemini` が、「どう待つか」を `veo` が持ちます。**

```go
client, err := gemini.NewClient(ctx, gemini.Config{ProjectID: "my-project", LocationID: "us-central1"})
if err != nil {
	return err
}

videoClient, err := veo.New(client,
	veo.WithPollInterval(10*time.Second),
	veo.WithPollTimeout(15*time.Minute),
)
if err != nil {
	return err
}

result, err := videoClient.Generate(ctx, "veo-3.1-generate-001", veo.Request{
	Prompt:       "a slow dolly-in on a coastal cliffside at dawn",
	Image:        &veo.Media{URI: "gs://bucket/keyframe.png", MIMEType: "image/png"},
	DurationSec:  8,
	AspectRatio:  "16:9",
	OutputGCSURI: "gs://bucket/videos/",
})
if err != nil {
	return err
}
video, _ := result.First() // video.URI に生成された動画の GCS URI が入ります
```

`Result` は `OperationName`（課金や失敗の追跡用）、`Videos`、`FilteredCount` / `FilteredReasons`（安全性ポリシーで除外された本数と理由）を持ちます。1 本も生成されなかった場合は `Generate` が `ErrNoVideoGenerated` を返すため、`FilteredCount` が非ゼロで `Videos` も非空なのは、複数本を要求して一部だけ除外されたケースです。

`veo.New` は `gemini.VideoGenerator`（`StartVideo` / `PollVideo` の 2 メソッド）を受け取るだけなので、テストでは genai SDK も GCP 認証も無しでポーリング挙動を検証できます。

### 入力の組み合わせ

Veo は入力系統を併用できません。`StartVideo` は API が確実に拒否する組み合わせを送信前に `ErrInvalidVideoInput` で弾きます。

| 使う機能 | 設定するフィールド |
| --- | --- |
| image-to-video | `Image` |
| first/last frame 補間 | `Image` + `LastFrame` |
| reference-to-video | `References`（`Image` / `Video` / `LastFrame` とは排他） |
| video extension（継続生成） | `Video`（`Image` とは排他） |

`References` に渡す `veo.Reference` は、画像の使われ方を `Type` で指定します。`gemini.VideoReferenceAsset`（被写体を登場させる。最大3枚）と `gemini.VideoReferenceStyle`（画風を反映させる）から選び、未指定は API のデフォルトに委ねます。

生成パラメータは `veo.Request` の以下のフィールドで指定します。ゼロ値はいずれも「API のデフォルトに委ねる」の意味です。

| フィールド | 役割 |
| --- | --- |
| `Prompt` | 生成指示。`Image` / `Video` のいずれも無い場合は必須です。 |
| `DurationSec` | 生成する動画の秒数。受け付けられる値はモデルと入力の組み合わせで異なります。 |
| `AspectRatio` / `Resolution` | `"16:9"` / `"9:16"`、`"720p"` / `"1080p"` などを指定します。 |
| `NegativePrompt` | 生成に含めたくない要素を指定します。 |
| `GenerateAudio` | 音声を同時生成するか（`*bool`）。nil で API のデフォルト。 |
| `Seed` | 再現性のためのシード（`*int64`）。`int32` の範囲外は `ErrInvalidSeed` です。 |
| `NumberOfVideos` | 生成する本数。0 で API のデフォルト（通常 1 本）。 |
| `OutputGCSURI` | 保存先の GCS バケット。未指定なら結果はバイト列で返ります（長尺では応答が大きくなるため通常は指定します）。 |

### ポーリングの持ち方

`gemini.PollVideo` は **1 回の問い合わせに徹し、リトライを掛けません**。ポーリング自体が繰り返しの仕組みなので、その内部でさらにバックオフを効かせると 1 回の問い合わせに数十秒かかりうる二重の待ちになり、設定したポーリング間隔とタイムアウトが意味を失うためです。一時的な失敗を何回まで許容するかは、間隔とタイムアウトを持っている `veo.Client` の判断で、`WithMaxPollErrors`（既定 10 回）で調整します。

一方、**投函（`StartVideo`）には `Config` のリトライ設定が効きます**。レート制限や一時的なサーバーエラーで 1 本分の生成が落ちるのを防ぐためです。

`Wait` は最初の問い合わせを間隔待ちなしで直ちに行います。別実行からの再開ではオペレーションが既に完了していることが多く、確認前に 1 interval 分（既定 10 秒）待つのは純粋な死に時間になるためです。

`Generate` は投函と完了待ちをまとめて行いますが、`Submit` と `Wait` に分けても呼べます。実行時間に上限のあるジョブ基盤で、投函だけ済ませて一旦戻り、次の実行でオペレーション名を渡して待ちを再開する、といった使い方ができます。

ログの出力先は `veo.WithLogger(*slog.Logger)` で差し替えられます（未指定は `slog.Default()`）。

```go
// 実行 1: 投函してオペレーション名を保存する（完了は待たない）
name, err := videoClient.Submit(ctx, "veo-3.1-generate-001", veo.Request{Prompt: "..."})

// 実行 2: 保存した名前で待ちを再開する
result, err := videoClient.Wait(ctx, name)
```

### SDK 未対応フィールドの送信

SDK がまだ型として持たないプレビュー機能は、2 つの方法でリクエストボディへ差し込めます。いずれも構造はバックエンドの REST API に一致させる必要があり、型検査も検証も効きません。SDK が対応している項目は通常のフィールドを使ってください。

| フィールド | 用途 |
| --- | --- |
| `Request.ExtraBody` | ボディへマージする値。**マージが再帰するのはマップ同士のときだけ**で、同じキーの値が配列なら丸ごと置き換わります |
| `Request.ModifyRequestBody` | 組み立て済みボディを受け取って書き換える関数。`ExtraBody` のマージ後に呼ばれます |

Vertex AI のリクエストは `{"instances": [...], "parameters": {...}}` という形なので、`instances` の要素へ値を足したい場合に `ExtraBody` を使うと **`instances` 配列ごと置き換わり、prompt も画像入力も消えます**。その用途では `ModifyRequestBody` を使ってください。

```go
req.ModifyRequestBody = func(body map[string]any) map[string]any {
	instances, _ := body["instances"].([]any)
	if len(instances) > 0 {
		if instance, ok := instances[0].(map[string]any); ok {
			instance["audio"] = map[string]any{"gcsUri": "gs://bucket/bgm.mp3"}
		}
	}
	return body
}
```

どちらも効くのは「SDK が既に叩いているエンドポイントのボディをいじる」場合だけです。エンドポイント自体が SDK に無い場合（Interactions API など）は救えません。

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
| `RequestTimeout` | 生成呼び出し 1 回（リトライ含む）の上限時間。File API とポーリングには適用されません | なし（無制限） |
| `AsyncCleanupTimeout` | アップロード後処理失敗時のバックグラウンド削除の上限時間 | `15s` |
| `Logger` | ライブラリ内部ログの出力先（`*slog.Logger`） | `slog.Default()` |
| `HTTPClient` | genai SDK が使う HTTP クライアント。タイムアウトやプロキシ、SSRF 対策済みクライアント（`securenet.NewSafeHTTPClient` 等）の注入に使います | SDK 既定 |
| `OnRetry` | リトライ直前に呼ばれる通知関数 | なし |

`APIKey` と `ProjectID` / `LocationID` は排他的です。Vertex AI を使う場合は `ProjectID` と `LocationID` の両方を指定してください。

`HTTPClient` を指定しても認証は失われません。genai SDK は HTTP クライアントを渡されると Application Default Credentials の検出をスキップし、認証ヘッダを付けずに送信してしまいますが（Vertex AI では全リクエストが 401 `CREDENTIALS_MISSING` になります）、本ライブラリが認証情報を付け直します。渡したインスタンス自体は変更せず、複製を使うため、同じクライアントを他の用途と共有しても構いません。

---

## 🧪 生成オプション (`gemini.GenerateOptions`)

ゼロ値が意味を持つ項目（`Temperature: 0` = 最も決定的、`ThinkingBudget: 0` = 思考無効）は、
「未設定」と区別するためポインタ型です。設定には `gemini.Ptr` ヘルパーを使います。

| 設定項目 | 役割 |
| --- | --- |
| `SystemPrompt` | System instruction を指定します。 |
| `Temperature` | 出力のランダム性（`*float32`）。`Ptr[float32](0)` で最も決定的。nil で SDK デフォルト。 |
| `TopP` / `TopK` | サンプリング範囲の制御（`*float32`）。nil で SDK デフォルト。 |
| `MaxOutputTokens` | 生成する最大トークン数。0 で SDK デフォルト。 |
| `StopSequences` | 生成を打ち切る文字列のリスト。 |
| `ThinkingBudget` | 思考トークンの上限（`*int32`）。`Ptr[int32](0)` で思考を無効化しコストとレイテンシを抑えます。nil でモデル既定。有効範囲はモデル依存です。 |
| `ThinkingLevel` | 思考量の段階指定（`gemini.ThinkingMinimal` / `ThinkingLow` / `ThinkingMedium` / `ThinkingHigh`）。モデル非依存で移植性が高い方の指定方法です。`ThinkingBudget` と併用した場合はこちらが優先されます。 |
| `IncludeThoughts` | true にすると思考サマリが `Response.Thoughts` に入ります（`Text` には含まれません）。 |
| `AspectRatio` | 画像生成時のアスペクト比を指定します。 |
| `ImageSize` | 画像生成時のサイズを指定します。 |
| `Seed` | 再現性のためのシード値。`int32` の範囲内である必要があります。 |
| `PersonGeneration` | Vertex AI 画像生成での人物生成ポリシーを指定します。 |
| `SafetySettings` | 安全フィルタの設定。`gemini.NewSafetySettings(gemini.SafetyBlockNone)` で構築できます。 |
| `ResponseMIMEType` | `image/png` や `audio/wav` など、期待するレスポンス MIME type を指定します。 |
| `ResponseSchema` | 構造化出力（constrained decoding）のスキーマ（`*gemini.Schema`）。`application/json` と併用すると、出力が文法レベルでスキーマに制約されます。 |
| `ResponseJSONSchema` | 標準的な JSON Schema による構造化出力。`$ref` を含む複雑なスキーマで `ResponseSchema` がうまく機能しない場合の代替です。併用した場合はこちらが優先されます。 |

標準的な4つのハームカテゴリ（暴力・ヘイト・性的表現・危険行為）すべてに同一の閾値を適用したい場合は、`gemini.NewSafetySettings(threshold)` ヘルパーを使うと `SafetySettings` を簡潔に構築できます。閾値をバックエンドや用途に応じてどう選ぶかは呼び出し側の判断に委ねています（Vertex AI は `SafetyOff` を受け付けません）。

```go
opts := gemini.GenerateOptions{
    SafetySettings: gemini.NewSafetySettings(gemini.SafetyBlockNone),
}
```

構造化出力のスキーマは `gemini.Schema` と `gemini.TypeObject` などの型定数で書けます（`genai.Schema` の別名なので、genai の値をそのまま渡すこともできます）。

```go
opts := gemini.GenerateOptions{
    ResponseMIMEType: "application/json",
    ResponseSchema: &gemini.Schema{
        Type: gemini.TypeObject,
        Properties: map[string]*gemini.Schema{
            "title":    {Type: gemini.TypeString},
            "keywords": {Type: gemini.TypeArray, Items: &gemini.Schema{Type: gemini.TypeString}},
        },
        Required: []string{"title"},
    },
}
```

`ResponseJSONSchema` の `map[string]any` と違い、フィールド名がコンパイル時に検査されます（`"propertise"` のような綴り間違いが黙って無視されません）。`$ref` を含むなど、この型で表現しきれないスキーマの場合だけ `ResponseJSONSchema` を選んでください。

閾値は `gemini.SafetyBlockNone` / `SafetyBlockLowAndAbove` / `SafetyBlockMediumAndAbove` / `SafetyBlockOnlyHigh` / `SafetyOff` から選べます。同様に思考量も `gemini.ThinkingMinimal` / `ThinkingLow` / `ThinkingMedium` / `ThinkingHigh` を用意しており、これらを使えば設定値を選ぶためだけに genai SDK を import する必要はありません。

### エラーの分類

生成失敗の理由は `errors.Is` / `errors.AsType` で判別できます。ブロックはリトライしても解決しないため、プロンプトの見直しが必要です。

```go
resp, err := client.GenerateContent(ctx, model, prompt)
switch {
case errors.Is(err, gemini.ErrBlocked):
    // 安全フィルタ等でブロックされた。詳細な理由は FinishReason を参照
    if apiErr, ok := errors.AsType[*gemini.APIResponseError](err); ok {
        slog.Warn("blocked", "reason", apiErr.FinishReason)
    }
case errors.Is(err, gemini.ErrEmptyResponse):
    // 候補が 1 件も返らなかった
}
```

### 構造化出力の後処理

`ResponseSchema` + `ResponseMIMEType: "application/json"` による構造化出力（constrained decoding）を使っても、モデルが完結した JSON の後に余分な閉じ括弧や説明テキストを継ぎ足すことが実際にあります。`json.Unmarshal` の前段で `gemini.CleanJSONResponse(raw)` を通すと、こうした末尾ノイズを除去・補正できます。トップレベルが配列（`[...]`）のスキーマにも対応しています。

```go
resp, err := client.GenerateWithAttachments(ctx, model, prompt, nil, opts)
// ...
var out MyStruct
jsonStr := gemini.CleanJSONResponse(resp.Text)
if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
    // ...
}
```

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
- `ErrInvalidAttachment`: 添付の指定が不正な場合（`Data` と `URI` の併用、`Data` に MIME type が無い場合）。
- `ErrEmptyOperationName`: オペレーション名が空の場合。
- `ErrInvalidVideoInput`: 動画生成の入力の組み合わせが API の受け付けないものだった場合。
- `ErrVideoGenerationFailed`: 動画生成のオペレーションが失敗として完了した場合（`VideoOperation.Failure` に載ります）。
- `ErrBlocked`: 安全フィルタ等により生成がブロックされた場合。詳細は `APIResponseError.FinishReason` を参照します。
- `ErrEmptyResponse`: 候補が 1 件も含まれないレスポンスが返された場合。

`ErrBlocked` / `ErrEmptyResponse` は `*APIResponseError` として返り、`Unwrap` がこれらのセンチネルを返すため `errors.Is` で分類できます。どちらも再試行では解決しないため、リトライ対象外です。

センチネルの文言は英語 + パッケージ名プレフィックス（`gemini:` / `veo:` / `lyria:`）で統一しています。深いラップの中に埋まってもどのパッケージ由来か判別でき、人間向けの文脈はラップする側が日本語で補う方針です。

`veo` パッケージは以下を公開しています。

- `veo.ErrGeneratorRequired`: `veo.New` に nil の生成クライアントを渡した場合。
- `veo.ErrMissingOperationName`: 完了待ちに必要なオペレーション名が無い場合。
- `veo.ErrNoVideoGenerated`: 成功で完了したのに動画が 1 本も返らなかった場合（安全性ポリシーによる除外が典型）。
- `veo.ErrPollFailed`: 生成状況の確認が連続して失敗し、完了を待てなくなった場合。

`lyria` パッケージは以下を公開しています。

- `lyria.ErrWorkflowConfig`: `lyria.New` に必要な依存やモデル名が欠けている場合。
- `lyria.ErrNilInput`: 生成に必要な入力（収集コンテンツ・歌詞・レシピ）が nil の場合。
- `lyria.ErrEmptyLyrics`: 生成された歌詞ドラフトの本文が空だった場合。
- `lyria.ErrNoAudio`: Lyria の呼び出しは成功したのに音声データが返らなかった場合。
- `lyria.ErrInvalidResponse`: モデル出力が期待する JSON として解釈できなかった場合。再生成で解決することがあります。

---

## 🔌 インターフェース

`*gemini.Client` はこれらをすべて満たします。利用側は必要な範囲だけに依存すると、モックが小さくなります。いずれのインターフェースも genai SDK の型をシグネチャに含みません。

| インターフェース | メソッド |
| --- | --- |
| `Generator` | `GenerateWithAttachments` |
| `BackendInspector` | `IsVertexAI` |
| `FileManager` | `UploadFile` / `DeleteFile` |
| `Model` | 上記 3 つ（生成・ファイル管理・バックエンド判定）の集合 |
| `VideoGenerator` | `StartVideo` / `PollVideo` |

生成だけが必要なら 1 メソッドの `Generator` に、参照画像をアップロードしてから添付として渡すような利用側は `Model` に依存してください。

---

## 📂 パッケージ構成

| パッケージ | 役割 |
| --- | --- |
| `github.com/shouni/go-gemini-client/gemini` | Gemini / Vertex AI クライアント、リトライ、File API、レスポンス抽出。 |
| `github.com/shouni/go-gemini-client/music` | 楽曲構成のデータ型（`music.Recipe` / `Section` / `LyricsDraft` / `AIModels`）。依存を持たない葉パッケージで、型だけが欲しい下流はこれだけを import できます。 |
| `github.com/shouni/go-gemini-client/lyria` | 歌詞生成 → 作曲レシピ生成 → Lyria 音声生成の 3 段。`lyria.New` は `gemini.Generator` を受け取ります。段の間に品質ゲートを挟むのは利用側の判断のため、一括実行の入口は用意していません。`lyria.MusicRecipe` などの型名は `music` パッケージの別名として残っています。 |
| `github.com/shouni/go-gemini-client/veo` | Veo 動画生成の投函と完了待ち。`veo.New` は `gemini.VideoGenerator` を受け取ります。 |

### 楽曲型 (`music`) と lyria ワークフロー

`music.Recipe` は楽曲の構成（セクション・歌詞・使用モデル・seed）を表す、このエコシステムで最も広く共有される型です。JSON タグは snake_case で、保存済みレシピ JSON との互換性を保っています。

```go
import "github.com/shouni/go-gemini-client/music"

var r music.Recipe
r.Sections = []music.Section{{Name: "Verse", Duration: 30}}
clone := r.Clone() // スライスやポインタも複製する深いコピー
```

ワークフロー（`lyria.Workflow`）は `GenerateLyrics` → `Compose` → `GenerateAudio` の 3 段を個別のメソッドとして公開します。段の間に構造検証などの品質ゲートを挟めるようにするためで、一括実行の入口は意図的にありません。

---

## 🤝 依存関係 (Dependencies)

- [google.golang.org/genai](https://pkg.go.dev/google.golang.org/genai) - Google Gemini 公式 SDK
- [shouni/netarmor](https://github.com/shouni/netarmor) - ネットワークセキュリティ & リトライ戦略

---

## 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。
