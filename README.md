# ✨ Go Gemini Client

[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/go-gemini-client)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/go-gemini-client)](https://github.com/shouni/go-gemini-client/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## 🎯 概要: Gemini APIのクライアントライブラリ

Go AI Client は、Go言語で Google **Gemini API** を利用するためのクライアントライブラリを提供します。

-----

## 💎 特徴と設計思想

### 🛡️ Stable Image Generation Pipeline 

Gemini API で画像を含むリクエスト（マルチモーダル生成）を行う際の最大の障壁である「Error 500 (Internal Error)」を徹底的に排除します。

### 🤖 堅牢なAIクライアント (`pkg/gemini`)

* **高度なリトライ戦略:** 指数バックオフによる自動復旧。セーフティフィルタによるブロック時は即時停止する賢いリトライロジック。
* **決定論的な制御:** シード値 (`Seed`) の固定により、再現性のある画像・テキスト生成をサポート。

---



### 詳細設定 (`gemini.Config`)

| 設定項目 | 役割 | デフォルト値 |
| --- | --- | --- |
| **`Temperature`** | 応答の創造性 | `0.7` |
| **`MaxRetries`** | 最大リトライ回数 | `1` |
| **`InitialDelay`** | リトライ開始時の待機時間 | `30s` |


---

## 📂 プロジェクト構造

| ディレクトリ | 役割 |
| --- | --- |
| `pkg/gemini` | **外部層**: Gemini APIとの通信、リトライ、決定論的パラメータ管理。 |

### 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。
