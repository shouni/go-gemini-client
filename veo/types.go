// Package veo は、Veo による動画生成を扱うクライアントを提供します。
//
// 動画生成は長時間実行オペレーションで、投函してから完了までポーリングし続ける必要が
// あります。gemini パッケージが持つのはその1往復ずつ（StartVideo / PollVideo）で、
// このパッケージが「どう待つか」——ポーリング間隔、タイムアウト、一時的な失敗を何回まで
// 許容するか——を受け持ちます。
//
// 依存は gemini.VideoBackend の注入だけで、genai SDK には触れません。テストでは
// 2メソッドのフェイクを渡せば、GCP 認証も HTTP も無しでポーリング挙動を検証できます。
package veo

import (
	"time"

	"github.com/shouni/go-gemini-client/gemini"
)

const (
	// DefaultPollInterval は、生成完了を確認する既定の間隔です。
	// Veo の生成は分単位で掛かるため、短くしても API 呼び出しが増えるだけです。
	DefaultPollInterval = 10 * time.Second
	// DefaultPollTimeout は、完了を待つ既定の上限です。
	DefaultPollTimeout = 15 * time.Minute
	// DefaultMaxPollErrors は、ポーリングの連続失敗を許容する既定の回数です。
	// 一時的なエラーで生成済みの動画を取り逃がさない一方、恒久的な障害では
	// タイムアウトまで無駄に待たずに打ち切ります。
	DefaultMaxPollErrors = 10
)

// Request は動画生成1本分の入力です。
//
// gemini.VideoRequest の別名で、入力の組み立てにこのパッケージ独自の型を覚え直す
// 必要をなくしています。どちらの名前で書いても同じ型です。
type Request = gemini.VideoRequest

// Reference は referenceImages に渡す参照画像1枚です（gemini.VideoReference の別名）。
type Reference = gemini.VideoReference

// Media は動画生成に渡す画像・動画の入力です（gemini.Attachment の別名）。
// URI（Vertex AI では gs://）かバイト列のどちらかを設定します。
type Media = gemini.Attachment

// Result は完了した動画生成の結果です。
type Result struct {
	// OperationName は生成に使われたオペレーションの識別子です。
	// 課金や失敗の追跡でログに残せるよう保持しています。
	OperationName string
	// Videos は生成された動画です。Request.OutputGCSURI を指定した場合は URI が、
	// 指定しなかった場合は Data が入ります。
	Videos []gemini.Attachment
	// FilteredCount は安全性ポリシーで除外された本数です。1本も生成されなかった
	// 場合は Generate がエラーを返すため、ここが非ゼロで Videos も非空なのは
	// 複数本を要求して一部だけ除外されたケースです。
	FilteredCount int32
	// FilteredReasons は除外された理由です。
	FilteredReasons []string
}

// First は最初に生成された動画を返します。1本だけ要求する通常の使い方で、
// 添字アクセスと境界チェックを毎回書かずに済むようにしています。
// 動画が無い場合は false を返します。
func (r *Result) First() (gemini.Attachment, bool) {
	if r == nil || len(r.Videos) == 0 {
		return gemini.Attachment{}, false
	}
	return r.Videos[0], true
}
