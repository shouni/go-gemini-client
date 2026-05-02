# ✨ Go Gemini Client

[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/go-gemini-client)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/go-gemini-client)](https://github.com/shouni/go-gemini-client/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Status](https://img.shields.io/badge/Status-Completed-brightgreen)](#)
[![Go Report Card](https://goreportcard.com/badge/github.com/shouni/go-gemini-client)](https://goreportcard.com/report/github.com/shouni/go-gemini-client)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/go-gemini-client.svg)](https://pkg.go.dev/github.com/shouni/go-gemini-client)
[![Status](https://img.shields.io/badge/Status-Completed-brightgreen)](#)

## 🎯 概要: Net Armor 統合型ハイブリッド Gemini クライアント

**Go Gemini Client** は、[shouni/netarmor](https://github.com/shouni/netarmor) をコアに採用した、**Google Gemini (Google AI & Vertex AI)** を安全かつ効率的に利用するためのライブラリです。

ひとつのインターフェースで、軽量な **Gemini API (Google AI Studio)** と、エンタープライズ向けの **Vertex AI (Google Cloud)** を切り替えて利用可能。特に GCS (Google Cloud Storage) とのシームレスな連携に最適化されています。

---

## 💎 特徴と設計思想

### 🤖 ハイブリッド・バックエンド・サポート

* **Dual Backend**: `APIKey` 方式（Google AI）と `ProjectID/LocationID` 方式（Vertex AI）の両方に対応。
* **Vertex AI 連携**: Cloud Run 等の環境で IAM 権限を利用した認証に対応。API Key の管理が不要になり、よりセキュアな運用が可能です。

### 🛡️ 堅牢な AI クライアント (`pkg/gemini`)

* **高度なリトライ戦略**: 指数バックオフによる自動復旧。セーフティフィルタによるブロックなど、リトライすべきでないエラーを識別して即時停止するインテリジェントなロジックを搭載。
* **決定論的な制御**: シード値 (`Seed`) の管理により、生成 AI 特有の揺らぎを制御し、再現性のある出力をサポート。
* **型安全なエラー判定**: `errors.Is` を用いた正確なエラーハンドリングが可能です。

### 📁 高度なリソース管理

* **GCS 直接参照 (Vertex AI)**: Vertex AI モードでは、`gs://` 形式の URI を直接プロンプトに含めることができ、事前のアップロードや待機ループが不要になります。
* **File API サポート (Gemini API)**: メディアデータのアップロードと `Active` 状態までの自動ポーリングをサポート。

---

## 🚀 クイックスタート

### クライアントの初期化

#### 1. Vertex AI モード (推奨: Cloud Run / GCS 連携)

```go
ctx := context.Background()
client, err := gemini.NewClient(ctx, gemini.Config{
    ProjectID:  "your-google-cloud-project-id",
    LocationID: "asia-northeast1",
    Temperature: genai.Ptr(0.7),
})

```

#### 2. Gemini API モード (API Key 方式)

```go
client, err := gemini.NewClient(ctx, gemini.Config{
    APIKey: "YOUR_GEMINI_API_KEY",
})

```

### マルチモーダル生成 (GCS URI 直接使用)

Vertex AI モードを利用すると、GCS 上の画像をダウンロードすることなく直接解析できます。

```go
parts := []*genai.Part{
    {
        FileData: &genai.FileData{
            URI:      "gs://my-bucket/character-design.jpg",
            MIMEType: "image/jpeg",
        },
    },
    {Text: "この画像に基づいて漫画の台本を作成してください"},
}

resp, err := client.GenerateWithParts(ctx, "gemini-3-pro-image-preview", parts, gemini.GenerateOptions{})

```

---

## ⚙️ 詳細設定 (`gemini.Config`)

| 設定項目 | 役割 | デフォルト値 |
| --- | --- | --- |
| **`APIKey`** | Gemini API キー (Google AI モード用) | - |
| **`ProjectID`** | Google Cloud プロジェクト ID (Vertex AI モード用) | - |
| **`LocationID`** | リージョン名 (Vertex AI モード用) | - |
| **`Temperature`** | 応答の創造性 (0.0 - 2.0) | `0.7` |
| **`MaxRetries`** | 最大リトライ回数 | `1` |
| **`InitialDelay`** | リトライ開始時の待機時間 | `30s` |
| **`MaxDelay`** | リトライ待機時間の上限 | `120s` |

---

## 📂 プロジェクト構造

| ディレクトリ | 役割 |
| --- | --- |
| `pkg/gemini` | **コア・クライアント**: 通信、リトライ、マルチモーダル制御、Backend 切り替え。 |

---

## 📜 エラーハンドリング

本ライブラリでは、以下のセンチネルエラーをエクスポートしています。

* `ErrConfigRequired`: APIKey または ProjectID/LocationID のいずれも設定されていない場合。
* `ErrEmptyPrompt`: プロンプトが空の場合。
* `ErrInvalidTemperature`: 温度設定が範囲外 (0.0 - 2.0) の場合。

---

## 🤝 依存関係 (Dependencies)

* [google.golang.org/genai](https://pkg.go.dev/google.golang.org/genai) - Google Gemini 公式 SDK
* [shouni/netarmor](https://github.com/shouni/netarmor) - **ネットワークセキュリティ & リトライ戦略**

---

### 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。

---
