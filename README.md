# ✨ Go Gemini Client

[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/go-gemini-client)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/go-gemini-client)](https://github.com/shouni/go-gemini-client/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## 🎯 概要: Net Armor統合型 Geminiクライアントライブラリ

**Go Gemini Client** は、[shouni/netarmor](https://github.com/shouni/netarmor) をコアに採用した、**Gemini API (Google GenAI SDK)** を 安全かつ効率的に利用するためのラッパーライブラリです。特にマルチモーダル生成（画像生成・理解）におけるリソース管理と、エンタープライズ用途に耐えうる堅牢なエラーハンドリングに特化しています。

-----

## 💎 特徴と設計思想

### 🛡️ 安定した画像生成パイプライン

Gemini API を利用した画像生成において、参照画像の一貫性を保つための **File API ライフサイクル管理** をネイティブにサポートします。

### 🤖 堅牢な AI クライアント (`pkg/gemini`)

* **高度なリトライ戦略**: 指数バックオフによる自動復旧。セーフティフィルタによるブロックなど、リトライすべきでないエラーを識別して即時停止するインテリジェントなロジックを搭載。
* **決定論的な制御**: シード値 (`Seed`) の管理により、生成 AI 特有の揺らぎを制御し、再現性のある出力をサポート。
* **型安全なエラー判定**: センチネルエラーの導入により、`errors.Is` を用いた正確なエラーハンドリングが可能です。

### 📁 高度なリソース管理

* **File API サポート**: 巨大なメディアデータを事前にアップロードし、`Active` 状態になるまで自動ポーリング。生成された URI を複数のリクエストで効率的に再利用できます。

---

## 🚀 クイックスタート

### クライアントの初期化

```go
ctx := context.Background()
client, err := gemini.NewClient(ctx, gemini.Config{
    APIKey:      "YOUR_GEMINI_API_KEY",
    Temperature: genai.Ptr(0.7),
    MaxRetries:  3,
})

```

### 画像リファレンスを使用した生成 (File API)

```go
// 1. 画像を File API にアップロード
uri, fileName, err := client.UploadFile(ctx, imageBytes, "image/png", "character-design")

// 2. アップロードした URI を使用して生成
parts := []*genai.Part{
    {Text: "このキャラクターのデザインを維持したまま、別のポーズを生成して"},
    {FileData: &genai.FileData{FileURI: uri}},
}
resp, err := client.GenerateWithParts(ctx, "imagen-3.0-generate-001", parts, gemini.GenerateOptions{
    AspectRatio: "16:9",
})

// 3. 最後にリソースをクリーンアップ
client.DeleteFile(ctx, fileName)

```

---

## ⚙️ 詳細設定 (`gemini.Config`)

| 設定項目 | 役割 | デフォルト値 |
| --- | --- | --- |
| **`APIKey`** | Gemini API キー (必須) | - |
| **`Temperature`** | 応答の創造性 (0.0 - 1.0) | `0.7` |
| **`MaxRetries`** | 最大リトライ回数 | `1` |
| **`InitialDelay`** | リトライ開始時の待機時間 | `30s` |
| **`MaxDelay`** | リトライ待機時間の上限 | `120s` |

---

## 📂 プロジェクト構造

| ディレクトリ | 役割 |
| --- | --- |
| `pkg/gemini` | **コア・クライアント**: 通信、リトライ、File API 管理、パラメータ制御。 |

---

## 📜 エラーハンドリング

本ライブラリでは、以下のセンチネルエラーをエクスポートしています。

* `ErrEmptyPrompt`: プロンプトが空の場合。
* `ErrAPIKeyRequired`: API キーが設定されていない場合。
* `ErrInvalidTemperature`: 温度設定が範囲外の場合。

## 🤝 依存関係 (Dependencies)

* [shouni/gemini-image-kit](https://github.com/shouni/gemini-image-kit) - Gemini 画像作成コア
* [shouni/netarmor](https://github.com/shouni/netarmor) - **ネットワークセキュリティ & リトライ戦略**

---

### 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。

---
