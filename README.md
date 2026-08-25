# ✨ Go Gemini Client

[![CI](https://github.com/shouni/go-gemini-client/actions/workflows/ci.yml/badge.svg)](https://github.com/shouni/go-gemini-client/actions/workflows/ci.yml)
[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/go-gemini-client)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/go-gemini-client)](https://github.com/shouni/go-gemini-client/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/go-gemini-client.svg)](https://pkg.go.dev/github.com/shouni/go-gemini-client)
[![Status](https://img.shields.io/badge/Status-Active-brightgreen)](#)

## 🎯 概要

**Go Gemini Client** は、**Google Gemini API / Vertex AI** 向けの Go ライブラリです。テキスト生成に加えて、GCS URI や File API を使ったマルチモーダル入力、画像・音声レスポンス、Lyria による音楽生成、Veo による動画生成を扱えます。

---

## 💎 設計の要点

- **genai SDK を公開 API に出しません。** 生成・構造化出力・安全設定・思考量・動画生成のいずれも SDK を import せずに書けます。利用側のモックも 1 メソッドで済みます。
- **ひとつのクライアントで 2 つのバックエンド。** `APIKey`（Gemini API）と `ProjectID` / `LocationID`（Vertex AI）を排他で受け取り、`gs://` の扱いなどバックエンド差は内部で吸収します。
- **失敗の種類を型で区別します。** 設定不備・入力不備・安全フィルタによるブロックはすべてセンチネルエラーで、`errors.Is` で分類できます。リトライしても直らない失敗は再試行しません。
- **長時間実行と重複呼び出しを引き受けます。** `veo` が動画生成のポーリングと一時的失敗の許容を、`callguard` が発射間隔・上限時間・同一内容の重複排除を持ちます。

---

## 📂 パッケージ構成

| パッケージ | 役割 |
| --- | --- |
| `github.com/shouni/go-gemini-client/gemini` | Gemini / Vertex AI クライアント、リトライ、File API、レスポンス抽出、Veo の 1 往復（`StartVideo` / `PollVideo`）。 |
| `github.com/shouni/go-gemini-client/music` | 楽曲構成のデータ型（`Recipe` / `Section` / `LyricsDraft` / `AIModels`）。依存を持たない葉パッケージです。 |
| `github.com/shouni/go-gemini-client/lyria` | 歌詞生成 → 作曲レシピ生成 → Lyria 音声生成の 3 段。`lyria.New` は `gemini.Generator` を受け取ります。 |
| `github.com/shouni/go-gemini-client/veo` | Veo 動画生成の投函と完了待ち。`veo.New` は `gemini.VideoGenerator` を受け取ります。 |
| `github.com/shouni/go-gemini-client/callguard` | AI 呼び出しへの発射間隔・1 回あたりの上限時間・重複排除（singleflight）。下流のキットが自前で持っていた実装の共通化先です。 |

### 楽曲型 (`music`) と lyria ワークフロー

`music.Recipe` は楽曲の構成（セクション・歌詞・使用モデル・seed）を表す、このエコシステムで最も広く共有される型です。JSON タグは snake_case で、保存済みレシピ JSON との互換性を保っています。

```go
import "github.com/shouni/go-gemini-client/music"

var r music.Recipe
r.Sections = []music.Section{{Name: "Verse", Duration: 30}}
clone := r.Clone() // スライスやポインタも複製する深いコピー
```

型だけを別パッケージへ切り出しているのは、レシピを読み書きするだけの下流サービスが、レート制限や singleflight を伴うワークフロー本体まで輸入せずに済むようにするためです。`lyria.MusicRecipe` / `MusicSection` / `LyricsDraft` / `AIModels` は `music` の型の別名なので、既存の表記もそのまま使えます。

ワークフロー（`lyria.Workflow`）は `GenerateLyrics` → `Compose` → `GenerateAudio` の 3 段を個別のメソッドとして公開します。段の間に構造検証などの品質ゲートを挟めるようにするためで、**一括実行の入口は意図的にありません**（品質ゲートは製品ごとに違うため、束ねても呼び出し側で分解し直すことになります）。

### 呼び出しガード (`callguard`)

高価な AI 呼び出しに、**発射間隔**（クォータ保護）・**1 回あたりの上限時間**・**同一内容の同時実行の重複排除**をまとめて掛けます。`lyria` が内部で使っているほか、`go-comic-kit` / `go-veo-orchestrator` のようにこのクライアントの上でワークフローを組むキットが、同じ機構を書き写さずに済むよう公開しています。

```go
guard := callguard.New(
    callguard.WithRateInterval(6*time.Second), // 毎分 10 回まで
    callguard.WithExecTimeout(5*time.Minute),
)

var group callguard.Group // ゼロ値で使えます

key := callguard.Key("image", model, prompt, callguard.SeedKey(seed))
resp, err := callguard.Do(ctx, &group, guard, key, func(execCtx context.Context) (*Response, error) {
    return inner.Generate(execCtx, req)
})
```

設計上の要点は 3 つです。

- **クォータはプロジェクト単位で、操作の種類ごとではありません。** テキスト生成と画像生成で別々に絞っても意味がないため、ワークフロー全体で `Guard` を 1 つ共有し、重複排除の単位（`Group`）だけを呼び出しの種類ごとに分けます。
- **発射間隔の待機は上限時間の外側です。** 待たされた時間を 1 回あたりの上限時間に数えると、混雑しているだけでタイムアウトします。
- **上限時間に「無制限」はありません。** 共有実行は呼び出し元の context から切り離されるため（リーダーの離脱が相乗り側を巻き添えにしないため）、これが唯一の打ち切り手段です。無制限にすると、応答の返らない 1 回が同じキーの後続を永久に待たせます。

戻り値は相乗りした全員で共有されます。呼び出し側が書き換える可能性があるものは複製してから返してください。

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

	resp, err := client.GenerateContent(ctx, "gemini-3.7-flash", "Goで短い俳句を書いて")
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

生成の入口は 2 つだけです。

| メソッド | 用途 |
| --- | --- |
| `GenerateContent(ctx, model, prompt)` | テキストのみ・オプション無しの最短経路 |
| `GenerateWithAttachments(ctx, model, prompt, attachments, opts)` | それ以外すべて（添付・構造化出力・安全設定・思考量・画像生成） |

`gemini.Attachment` はバイト列と URI 参照のどちらも表現できます。

```go
resp, err := client.GenerateWithAttachments(ctx, "gemini-3.7-flash",
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
- プロンプトが空でも添付があれば送信できます（音声だけを渡して解析させる用途）。両方が空の場合は `ErrEmptyParts` です
- 添付が無いテキスト生成でも使えます。`attachments` に `nil` を渡せば「プロンプト + `GenerateOptions`」の入口になります

プロンプトは添付より前に置かれます。**`genai.Part` を直接受け取る公開 API は意図的にありません。** SDK の型を公開面へ漏らすと、利用側が genai を import する理由が復活してしまうためです。

---

## 🖼️ 画像・音声レスポンス

`ResponseMIMEType` に `image/*` または `audio/*` を指定すると、レスポンスモダリティが自動設定されます。

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
	_ = attachment.MIMEType // 保存時の拡張子や Content-Type の決定に使えます
}
```

### `gemini.Response` の中身

| フィールド | 内容 |
| --- | --- |
| `Text` | 本文。思考パートを除く全テキストパートの連結です。 |
| `Images` / `Audios` | インラインデータのバイト列を MIME type で振り分けたものです。 |
| `Attachments` | インラインデータを MIME type 付きで返却順に保持します（`Images` / `Audios` の上位集合）。バイト列だけでは保存時の拡張子や Content-Type を決められないため、型を保ったまま取り出せる形を用意しています。 |
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
defer func() {
	// 削除に失敗するとサーバー側にファイルが残るため、握りつぶさず記録します。
	if err := client.DeleteFile(context.Background(), uploaded.Name); err != nil {
		slog.Warn("failed to delete uploaded file", "name", uploaded.Name, "error", err)
	}
}()

resp, err := client.GenerateWithAttachments(ctx, "gemini-3.7-flash",
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

## ⚙️ 詳細設定 (`gemini.Config`)

| 設定項目 | 役割 | デフォルト値 |
| --- | --- | --- |
| `APIKey` | Gemini API キー。Google AI Studio / Gemini API で利用します。 | - |
| `ProjectID` | Google Cloud プロジェクト ID。Vertex AI で利用します。 | - |
| `LocationID` | Vertex AI のリージョン。例: `asia-northeast1`, `us-central1` | - |
| `MaxRetries` | 最大リトライ回数（`*uint64`）。`nil` は既定値、`new(uint64(0))` は再試行しない | `1` |
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

ゼロ値が意味を持つ項目（`Temperature: 0` = 最も決定的、`ThinkingBudget: 0` = 思考無効）は、「未設定」と区別するためポインタ型です。設定には Go 1.26 の `new(式)` を使います（`new(float32(0))`）。`gemini.Ptr` は互換のため残していますが、`go fix` で `new` へ置き換わります。

| 設定項目 | 役割 |
| --- | --- |
| `SystemPrompt` | System instruction を指定します。 |
| `Temperature` | 出力のランダム性（`*float32`）。`new(float32(0))` で最も決定的。nil で SDK デフォルト。 |
| `TopP` / `TopK` | サンプリング範囲の制御（`*float32`）。nil で SDK デフォルト。 |
| `MaxOutputTokens` | 生成する最大トークン数。0 で SDK デフォルト。 |
| `StopSequences` | 生成を打ち切る文字列のリスト。 |
| `ThinkingBudget` | 思考トークンの上限（`*int32`）。`new(int32(0))` で思考を無効化しコストとレイテンシを抑えます。nil でモデル既定。有効範囲はモデル依存です。 |
| `ThinkingLevel` | 思考量の段階指定。モデル非依存で移植性が高い方の指定方法で、`ThinkingBudget` と併用した場合はこちらが優先されます。 |
| `IncludeThoughts` | true にすると思考サマリが `Response.Thoughts` に入ります（`Text` には含まれません）。 |
| `AspectRatio` / `ImageSize` | 画像生成時のアスペクト比とサイズ。 |
| `Seed` | 再現性のためのシード値。`int32` の範囲内である必要があります。 |
| `PersonGeneration` | Vertex AI 画像生成での人物生成ポリシーを指定します。 |
| `SafetySettings` | 安全フィルタの設定。 |
| `ResponseMIMEType` | `image/png` や `audio/wav` など、期待するレスポンス MIME type を指定します。 |
| `ResponseSchema` | 構造化出力（constrained decoding）のスキーマ（`*gemini.Schema`）。`application/json` と併用すると、出力が文法レベルでスキーマに制約されます。 |
| `ResponseJSONSchema` | 標準的な JSON Schema（`map[string]any`）による構造化出力。`$ref` を含むなど `ResponseSchema` で表現しきれない場合の代替で、併用した場合はこちらが優先されます。 |

### genai を import せずに値を選ぶ

設定値の定数はすべて再エクスポートしてあるため、値を選ぶためだけに genai SDK を import する必要はありません。

| 種別 | 選べる値 |
| --- | --- |
| 安全フィルタ閾値（`SafetyThreshold`） | `SafetyBlockNone` / `SafetyBlockLowAndAbove` / `SafetyBlockMediumAndAbove` / `SafetyBlockOnlyHigh` / `SafetyOff`（Vertex AI は `SafetyOff` を受け付けません） |
| 思考量（`ThinkingLevel`） | `ThinkingMinimal` / `ThinkingLow` / `ThinkingMedium` / `ThinkingHigh`（`ThinkingUnspecified` でモデル既定） |
| 人物生成（`PersonGeneration`） | `PersonGenerationAllowAll` / `PersonGenerationAllowAdult` / `PersonGenerationDontAllow`（未指定は API 既定） |
| スキーマ型（`SchemaType`） | `TypeString` / `TypeNumber` / `TypeInteger` / `TypeBoolean` / `TypeArray` / `TypeObject` |
| 動画参照の種別（`VideoReferenceType`） | `VideoReferenceAsset` / `VideoReferenceStyle` |

標準的な 4 つのハームカテゴリ（暴力・ヘイト・性的表現・危険行為）すべてに同一の閾値を適用する場合は `NewSafetySettings` を使います。

```go
opts := gemini.GenerateOptions{
    SafetySettings: gemini.NewSafetySettings(gemini.SafetyBlockNone),
}
```

構造化出力のスキーマは `gemini.Schema`（`genai.Schema` の別名）で書けます。`ResponseJSONSchema` の `map[string]any` と違い、フィールド名がコンパイル時に検査されます（`"propertise"` のような綴り間違いが黙って無視されません）。

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

### 構造化出力の後処理

`ResponseSchema` + `ResponseMIMEType: "application/json"` による構造化出力（constrained decoding）を使っても、モデルは次の形で JSON を崩すことがあります。**どれも応答を返しきったあとの話なので、API の再試行では直りません。** `json.Unmarshal` の前段で `gemini.CleanJSONResponse(raw)` を通してください。

- Markdown のフェンス（```` ```json … ``` ````）で包む
- 完結した JSON の後ろに説明文や余分な閉じ括弧を継ぎ足す
- `}` の代わりに `)` などで閉じる
- 文字列の中でバックスラッシュをエスケープし忘れる（正規表現やパスを引用したとき）
- 文字列の中に改行やタブを生のまま入れる（複数行の本文を引用したとき）

後ろの 2 つは、**台本の抜粋・歌詞・台詞のように複数行の本文を JSON に載せる用途で特に起きます。** トップレベルが配列（`[...]`）のスキーマにも対応しています。

**既に解釈できる入力は 1 バイトも変えません。** 補修しても妥当な JSON にならなければ入力をそのまま返すので、呼び出し側のエラーメッセージは元の壊れ方を指したままになります。

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

センチネルの文言は英語 + パッケージ名プレフィックス（`gemini:` / `veo:` / `lyria:`）で統一しています。深いラップの中に埋まってもどのパッケージ由来か判別でき、人間向けの文脈はラップする側が日本語で補う方針です。

### 生成失敗の分類

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

`ErrBlocked` / `ErrEmptyResponse` は `*APIResponseError` として返り、`Unwrap` がこれらのセンチネルを返すため `errors.Is` で分類できます。どちらも再試行では解決しないため、リトライ対象外です。

### センチネル一覧

**`gemini`** — 設定不備:

- `ErrConfigRequired`: `APIKey` と `ProjectID` / `LocationID` のいずれも設定されていない場合。
- `ErrExclusiveConfig`: `APIKey` と `ProjectID` / `LocationID` が同時に設定されている場合。
- `ErrIncompleteVertexConfig`: `ProjectID` または `LocationID` の片方だけが設定されている場合。

**`gemini`** — 入力検証:

- `ErrEmptyPrompt`: プロンプトが空の場合（`GenerateContent`）。
- `ErrEmptyModelName`: モデル名が空の場合。
- `ErrEmptyParts`: プロンプトと添付の両方が空で、送るものが何も無い場合。
- `ErrInvalidAttachment`: 添付の指定が不正な場合（`Data` と `URI` の併用、`Data` に MIME type が無い場合）。
- `ErrInvalidSeed`: `Seed` が `int32` の範囲外の場合。
- `ErrInvalidPart`: 生成パーツに nil が含まれている場合。パーツは内部でのみ組み立てるため、公開 API 経由では発生しない内部ガードです。

**`gemini`** — レスポンス / 動画:

- `ErrBlocked`: 安全フィルタ等により生成がブロックされた場合。
- `ErrEmptyResponse`: 候補が 1 件も含まれないレスポンスが返された場合。
- `ErrEmptyOperationName`: オペレーション名が空の場合。
- `ErrInvalidVideoInput`: 動画生成の入力の組み合わせが API の受け付けないものだった場合。
- `ErrVideoGenerationFailed`: 動画生成のオペレーションが失敗として完了した場合（`VideoOperation.Failure` に載ります）。

**`veo`**:

- `ErrGeneratorRequired`: `veo.New` に nil の生成クライアントを渡した場合。
- `ErrMissingOperationName`: 完了待ちに必要なオペレーション名が無い場合。
- `ErrNoVideoGenerated`: 成功で完了したのに動画が 1 本も返らなかった場合（安全性ポリシーによる除外が典型）。
- `ErrPollFailed`: 生成状況の確認が連続して失敗し、完了を待てなくなった場合。

**`lyria`**:

- `ErrWorkflowConfig`: `lyria.New` に必要な依存やモデル名が欠けている場合。
- `ErrNilInput`: 生成に必要な入力（収集コンテンツ・歌詞・レシピ）が nil の場合。
- `ErrEmptyLyrics`: 生成された歌詞ドラフトの本文が空だった場合。
- `ErrNoAudio`: Lyria の呼び出しは成功したのに音声データが返らなかった場合。
- `ErrInvalidResponse`: モデル出力が期待する JSON として解釈できなかった場合。再生成で解決することがあります。

---

## 🔌 インターフェース

`*gemini.Client` はこれらをすべて満たします。利用側は必要な範囲だけに依存すると、モックが小さくなります。**いずれも genai SDK の型をシグネチャに含みません。**

| インターフェース | メソッド |
| --- | --- |
| `Generator` | `GenerateWithAttachments` |
| `BackendInspector` | `IsVertexAI` |
| `FileManager` | `UploadFile` / `DeleteFile` |
| `Model` | 上記 3 つ（生成・ファイル管理・バックエンド判定）の集合 |
| `VideoGenerator` | `StartVideo` / `PollVideo` |

生成だけが必要なら 1 メソッドの `Generator` に、参照画像をアップロードしてから添付として渡すような利用側は `Model` に依存してください。

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

```go
// 実行 1: 投函してオペレーション名を保存する（完了は待たない）
name, err := videoClient.Submit(ctx, "veo-3.1-generate-001", veo.Request{Prompt: "..."})

// 実行 2: 保存した名前で待ちを再開する
result, err := videoClient.Wait(ctx, name)
```

`veo.Client` のオプションは `WithPollInterval`（既定 10 秒）/ `WithPollTimeout`（既定 15 分）/ `WithMaxPollErrors`（既定 10 回）/ `WithLogger`（既定 `slog.Default()`）です。

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

## 🤝 依存関係 (Dependencies)

- [google.golang.org/genai](https://pkg.go.dev/google.golang.org/genai) - Google Gemini 公式 SDK
- [shouni/netarmor](https://github.com/shouni/netarmor) - リトライ戦略（`retry`）とネットワークセキュリティ
- [golang.org/x/oauth2](https://pkg.go.dev/golang.org/x/oauth2) - `Config.HTTPClient` へ認証情報を付け直すために使用
- [golang.org/x/sync](https://pkg.go.dev/golang.org/x/sync) - `callguard` の singleflight
- [golang.org/x/time](https://pkg.go.dev/golang.org/x/time) - `callguard` のレート制限

---

## 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。
