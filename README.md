# ✨ Go Gemini Client

[![CI](https://github.com/shouni/go-gemini-client/actions/workflows/ci.yml/badge.svg)](https://github.com/shouni/go-gemini-client/actions/workflows/ci.yml)
[![Status](https://img.shields.io/badge/Status-Active-brightgreen)](#)
[![Language](https://img.shields.io/badge/Language-Go-blue)](https://go.dev/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/go-gemini-client)](https://go.dev/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/go-gemini-client)](https://github.com/shouni/go-gemini-client/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/go-gemini-client.svg)](https://pkg.go.dev/github.com/shouni/go-gemini-client)

## 🚀 概要 (About) - genai SDK を公開 API に出さない Gemini / Vertex クライアント。保存先は決めません

**Go Gemini Client** は、**Google Gemini API / Vertex AI** 向けの Go ライブラリです。テキスト生成、
GCS URI や File API を使ったマルチモーダル入力、画像・音声レスポンス、Lyria による音楽生成、
Veo による動画生成を扱います。生成物の保存先は決めず、参照画像の取得・再圧縮を伴う画像生成は
[gemini-image-kit](https://github.com/shouni/gemini-image-kit) が担当します。

Vertex AI だけで足りる系統は姉妹ライブラリの
[genai-kit](https://github.com/shouni/genai-kit) が担当します。
**使い分けの表は [genai-kit の README](https://github.com/shouni/genai-kit#-go-gemini-client-との使い分け)
にあります**（両方に置くと必ず片方が古くなるため、後発の側に 1 つだけ置いています）。

シグネチャ・フィールド・エラーの一覧は
[pkg.go.dev](https://pkg.go.dev/github.com/shouni/go-gemini-client) にあります。ここに書くのは、
godoc を読んでも気付けないことだけです。

---

## ✨ 提供機能 (Features)

* **`gemini`**: Gemini / Vertex AI クライアント。生成の入口は `GenerateContent`（テキストのみの
  最短経路）と `GenerateWithAttachments`（それ以外すべて）の 2 つだけです。
  * **ひとつのクライアントで 2 つのバックエンド。** `APIKey`（Gemini API）と `ProjectID` /
    `LocationID`（Vertex AI）は排他で、`gs://` の扱いなどバックエンド差は内部で吸収します。
    片方だけの指定は `ErrIncompleteVertexConfig`、どちらも無ければ `ErrConfigRequired` です。
  * **`genai.Part` を直接受け取る公開 API はありません。** 添付は `Attachment`（`MIMEType` と、
    `Data` か `URI` の片方）で表します。SDK の型を公開面へ出すと、利用側が genai を import する
    理由が復活してしまうためです。設定値の型と定数は別名として再エクスポートしてあるので、
    値を選ぶためだけの import も要りません（**Vertex AI は `SafetyOff` を受け付けません**。
    そこでは `SafetyBlockNone` を使ってください）。
  * **空の添付は黙って読み飛ばします。** 参照画像を「あれば渡す」形で組み立てる呼び出し側が、
    空要素の除去を毎回書かずに済みます。プロンプトが空でも添付があれば送れます（音声だけを
    渡して解析させる用途）。両方空なら `ErrEmptyParts` です。
  * **`Config.HTTPClient` を渡しても認証は失われません。** genai は HTTP クライアントを渡されると
    ADC の検出をスキップし、認証ヘッダ無しで送るため、素の `&http.Client{Timeout: ...}` だと
    Vertex AI への全リクエストが 401 `CREDENTIALS_MISSING` になります。本ライブラリが認証情報を
    付け直し、渡したインスタンスは書き換えず複製を使います。
  * **リトライは genai SDK に任せています。** 408 / 429 / 5xx と通信エラーの判定表を持たないので、
    SDK が対象を増やせばそのまま追随します。`Config` で回数と間隔を渡すだけです。
  * **構造化出力でも `CleanJSONResponse` を通してください。** `ResponseSchema` を指定しても、
    モデルは完結した JSON の後ろに説明文を継ぎ足したり、複数行の本文の中に生の改行を入れたりします。
    **どれも応答を返しきったあとの話なので、API の再試行では直りません。**
  * File API については[アップロードと Active 化待ち](#-file-api-のアップロードと-active-化待ち)。
* **`music`**: 楽曲構成のデータ型（`Recipe` / `Section` / `LyricsDraft` / `AIModels`）。依存を持たない
  葉パッケージで、レシピを読み書きするだけの下流サービスがワークフロー本体を輸入せずに済みます。
  JSON タグは snake_case で、**保存済みレシピ JSON との互換性の契約**です。
* **`lyria`**: 歌詞生成 → 作曲レシピ生成 → Lyria 音声生成の 3 段。
  * **プロンプト本文を一切持ちません。** 組み立ては `TextPromptGenerator` / `AudioPromptBuilder` を
    注入して決めます。
  * **一括実行の入口は意図的にありません。** 3 段を個別に呼ぶのは、段の間に構造検証などの
    品質ゲートを挟めるようにするためです。
  * `lyria.MusicRecipe` / `MusicSection` / `LyricsDraft` / `AIModels` は `music` の型の別名なので、
    既存の表記もそのまま使えます。
* **`veo`**: Veo 動画生成の投函と完了待ち。**「1 往復ずつ」を `gemini` が、「どう待つか」を `veo` が
  持ちます。**
  * `Submit`（投函だけ）と `Wait`（名前を渡して待ちを再開）に分けて呼べます。実行時間に上限のある
    ジョブ基盤で、投函を済ませて一旦戻る使い方ができます。
  * **投函にはリトライが効き、1 回ごとのポーリングには効きません。** ポーリングの中でさらに
    バックオフを効かせると、設定した間隔とタイムアウトが意味を失うためです。一時的な失敗は
    `WithMaxPollErrors` の回数まで受け流します。
  * **入力系統は併用できません**（image / video / references）。API が確実に拒否する組み合わせは
    送信前に `ErrInvalidVideoInput` で弾きます。
  * **`Request.ExtraBody` は配列の中へ届きません。** Vertex AI のリクエストは
    `{"instances": [...], "parameters": {...}}` の形で、マージが再帰するのはマップ同士のときだけ
    なので、`instances` を指定すると **配列ごと置き換わり prompt も画像入力も消えます**。
    その用途では `Request.ModifyRequestBody` を使ってください。どちらも効くのは「SDK が既に
    叩いているエンドポイントのボディをいじる」場合だけです。
* **`callguard`**: AI 呼び出しへの発射間隔・1 回あたりの上限時間・重複排除（singleflight）。
  * **クォータはプロジェクト単位で、操作の種類ごとではありません。** テキスト生成と画像生成で
    別々に絞っても意味がないため、ワークフロー全体で `Guard` を 1 つ共有し、重複排除の単位
    （`Group`）だけを呼び出しの種類ごとに分けます。
  * **戻り値は相乗りした全員で共有されます。** 呼び出し側が書き換える可能性があるものは
    複製してから返してください。

**genai SDK を import するのは `gemini` だけです。** 上位のパッケージはいずれも `gemini` の
1〜2 メソッドのインターフェース（`Generator` / `Model` / `FileManager` / `BackendInspector` /
`VideoGenerator`）だけを受け取るため、テストでは SDK も GCP 認証も要りません。利用側も、
生成だけが必要なら 1 メソッドの `Generator` に依存すればモックが 1 つで済みます。

---

## 📦 パッケージ構成 (Package Structure)

```text
go-gemini-client/
├── gemini/      # Gemini / Vertex AI クライアント。生成・リトライ・File API・Veo の 1 往復
├── music/       # 楽曲構成のデータ型。依存を持たない葉
├── lyria/       # 歌詞 → レシピ → 音声の 3 段
├── veo/         # Veo 動画生成の投函と完了待ち
├── callguard/   # 発射間隔・上限時間・重複排除。依存を持たない葉
└── internal/
    └── poll/    #   File API の Active 化待ちと veo が共有するポーリングの骨格
```

インポートパスはいずれも `github.com/shouni/go-gemini-client/` を前置します。

---

## 🚦 使い方 (Usage)

```sh
go get github.com/shouni/go-gemini-client
```

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

	// Vertex AI を使う場合は APIKey の代わりに ProjectID と LocationID を渡します
	// （両者は排他です）。Cloud Run などでは API キーを持たせずに運用できます。
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

添付付きの生成・画像生成・音楽生成・動画生成の例は
[pkg.go.dev](https://pkg.go.dev/github.com/shouni/go-gemini-client) にあります。

---

## 📤 File API のアップロードと Active 化待ち

`UploadFile` はアップロード後、ファイルが `Active` になるまで自動で待機します。返るのは
`UploadedFile{URI, Name}` で、`URI` は生成リクエストから参照する値、`Name` は `DeleteFile` に
渡す識別子です。

- **Upload はリトライされません。** genai SDK が再開可能アップロードの経路にだけリトライを
  掛けないためで、再試行するかは呼び出し側の判断です。Delete はリトライ対象で、対象が既に
  存在しない場合を成功として扱います
- **`Config.FilePollingTimeout`（既定 60 秒）は待機全体に掛かり、実行中のステータス確認 1 回にも
  同じ期限が渡ります。** genai の既定 HTTP クライアントにタイムアウトが無いため、確認の合間だけ
  見張る実装では応答の返らない 1 回で止まったままになります。確認自体にはリトライを掛けず、
  一時的な失敗をループ側で受け流します（`Config.FilePollingInterval` は既定 2 秒）
- **削除の失敗は握りつぶさず記録してください。** サーバー側にファイルが残ります。Active 化に
  失敗したときのバックグラウンド削除は投げっぱなしで、完了を待つ手段はありません
  （上限時間は `Config.AsyncCleanupTimeout`、既定 15 秒）

```go
uploaded, err := client.UploadFile(ctx, f, "video/mp4", "movie.mp4")
if err != nil {
	return err
}
defer func() {
	if err := client.DeleteFile(context.Background(), uploaded.Name); err != nil {
		slog.Warn("failed to delete uploaded file", "name", uploaded.Name, "error", err)
	}
}()
```

---

## 🤝 依存関係 (Dependencies)

- [google.golang.org/genai](https://pkg.go.dev/google.golang.org/genai) - Google Gemini 公式 SDK
- [golang.org/x/oauth2](https://pkg.go.dev/golang.org/x/oauth2) - `Config.HTTPClient` へ認証情報を付け直すために使用
- [golang.org/x/sync](https://pkg.go.dev/golang.org/x/sync) - `callguard` の singleflight
- [golang.org/x/time](https://pkg.go.dev/golang.org/x/time) - `callguard` のレート制限

---

## 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。
