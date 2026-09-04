package lyria

import (
	"context"
	"fmt"
	"strings"

	"github.com/shouni/go-gemini-client/callguard"
	"github.com/shouni/go-gemini-client/gemini"
)

// lyriaAudioGenerator は MusicRecipe を Lyria に渡し、音声を生成します。
type lyriaAudioGenerator struct {
	aiClient          gemini.Generator
	promptBuilder     AudioPromptBuilder
	defaultLyriaModel string
	// guard は nil で「制限なし・既定の上限時間」を意味します。
	guard *callguard.Guard
	group callguard.Group
}

// GenerateAudio は MusicRecipe 全体を 1 回の Lyria 呼び出しで音声化します。
func (g *lyriaAudioGenerator) GenerateAudio(ctx context.Context, recipe *MusicRecipe, images []ImagePayload) (*Track, error) {
	if recipe == nil {
		return nil, fmt.Errorf("%w: music recipe", ErrNilInput)
	}

	targetModel := g.defaultLyriaModel
	if recipe.AudioModel != "" {
		targetModel = recipe.AudioModel
	}

	promptText := g.promptBuilder.BuildFullSong(recipe)
	imageHash := calculateImagesHash(images)
	key := callguard.Key("audio-full", targetModel, promptText, callguard.SeedKey(recipe.Seed), imageHash)
	track, err := callguard.Do(ctx, &g.group, g.guard, key, func(execCtx context.Context) (*Track, error) {
		resp, err := g.aiClient.GenerateWithAttachments(
			execCtx,
			targetModel,
			promptText,
			images,
			buildAudioGenerateOptions(recipe.Seed),
		)
		if err != nil {
			return nil, fmt.Errorf("lyria generation failed (model: %s): %w", targetModel, err)
		}
		audio, ok := firstAudioAttachment(resp)
		if !ok {
			return nil, fmt.Errorf("%w (model: %s)", ErrNoAudio, targetModel)
		}

		return &Track{
			Audio:      audio.Data,
			MIMEType:   audio.MIMEType,
			SungLyrics: resp.Text,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	return track.Clone(), nil
}

// firstAudioAttachment は、レスポンスから最初の音声添付を MIME type ごと取り出します。
//
// Response.Audios ではなく Attachments を見るのは、MIME type を保つためです。Audios は
// バイト列だけなので、そこから取ると保存時の拡張子や Content-Type を呼び出し側が内容から
// 推測し直すことになります。Audios は Attachments を同じ接頭辞判定で絞った部分集合を
// 返却順のまま並べたものなので、選ばれる要素は変わりません。
func firstAudioAttachment(resp *gemini.Response) (gemini.Attachment, bool) {
	if resp == nil {
		return gemini.Attachment{}, false
	}
	for _, attachment := range resp.Attachments {
		if strings.HasPrefix(attachment.MIMEType, "audio/") {
			return attachment, true
		}
	}
	return gemini.Attachment{}, false
}
