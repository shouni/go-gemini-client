package lyria

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shouni/go-gemini-client/gemini"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"google.golang.org/genai"
)

// staticPromptGen はテスト用の固定プロンプト生成器です。
type staticPromptGen struct {
	lyricsPrompt string
	recipePrompt string
}

// GenerateLyrics は TextPromptGenerator インターフェースに合わせる
func (g staticPromptGen) GenerateLyrics(_ string, _ string) (string, error) {
	return g.lyricsPrompt, nil
}

// GenerateRecipe も同様にインターフェースに合わせる
func (g staticPromptGen) GenerateRecipe(_ string, _ *LyricsDraft) (string, error) {
	return g.recipePrompt, nil
}

// GenerateCoverArt も同様にインターフェースに合わせる
func (g staticPromptGen) GenerateCoverArt(_ string, _ *MusicRecipe) (string, error) {
	return g.recipePrompt, nil
}

type blockingGeminiClient struct {
	contentCalls atomic.Int32
	partsCalls   atomic.Int32

	contentStarted sync.Once
	partsStarted   sync.Once

	contentStartedCh chan struct{}
	partsStartedCh   chan struct{}
	releaseContentCh chan struct{}
	releasePartsCh   chan struct{}

	contentResp *gemini.Response
	partsResp   *gemini.Response
}

func newBlockingGeminiClient() *blockingGeminiClient {
	return &blockingGeminiClient{
		contentStartedCh: make(chan struct{}),
		partsStartedCh:   make(chan struct{}),
		releaseContentCh: make(chan struct{}),
		releasePartsCh:   make(chan struct{}),
	}
}

func (c *blockingGeminiClient) GenerateContent(context.Context, string, string) (*gemini.Response, error) {
	c.contentCalls.Add(1)
	c.contentStarted.Do(func() { close(c.contentStartedCh) })
	<-c.releaseContentCh
	return c.contentResp, nil
}

func (c *blockingGeminiClient) GenerateWithParts(context.Context, string, []*genai.Part, gemini.GenerateOptions) (*gemini.Response, error) {
	c.partsCalls.Add(1)
	c.partsStarted.Do(func() { close(c.partsStartedCh) })
	<-c.releasePartsCh
	return c.partsResp, nil
}

func (c *blockingGeminiClient) IsVertexAI() bool {
	return false
}

func TestLyriaLyricistSingleflightDeduplicatesConcurrentCalls(t *testing.T) {
	ctx := context.Background()
	client := newBlockingGeminiClient()
	client.partsResp = &gemini.Response{
		Text: `{"title":"Song","theme":"Theme","lyrics":"Words","keywords":["one"]}`,
	}

	lyricist := &lyriaTextGenerator{
		aiClient:     client,
		promptGen:    staticPromptGen{lyricsPrompt: "lyrics prompt"},
		defaultModel: "gemini-flash",
	}

	// 修正：文字列ではなく *CollectedContent を渡す
	input := &CollectedContent{
		Prompt: "same input",
	}

	const callers = 5
	results := make([]*LyricsDraft, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := range callers {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = lyricist.GenerateLyrics(ctx, AIModels{
				TextModel:   "gemini-flash",
				ComposeMode: "default",
			}, input)
		}(i)
	}

	require.Eventually(t, func() bool {
		select {
		case <-client.partsStartedCh:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)

	time.Sleep(20 * time.Millisecond)
	close(client.releasePartsCh)
	wg.Wait()

	require.Equal(t, int32(1), client.partsCalls.Load())
	for _, err := range errs {
		require.NoError(t, err)
	}

	// ディープコピーの検証（shared reference による事故を防いでいるか）
	require.NotSame(t, results[0], results[1])
	results[0].Keywords[0] = "changed"
	assert.Equal(t, "one", results[1].Keywords[0])
}

func TestCloneMusicRecipeDeepCopiesPointerFields(t *testing.T) {
	t.Parallel()

	seed := int64(7)
	src := &MusicRecipe{
		Title:        "Song",
		Key:          "A minor",
		VocalProfile: "Japanese female vocal, clear diction",
		Instruments:  []string{"synth"},
		Sections: []MusicSection{
			{Name: "Verse", Duration: 30, StartSeconds: 10, EndSeconds: 40, Prompt: "pulse"},
		},
		Lyrics: &LyricsDraft{
			Lyrics:   "words",
			Keywords: []string{"one"},
		},
		AIModels: AIModels{Seed: &seed},
	}

	cloned := cloneMusicRecipe(src)
	require.NotNil(t, cloned)
	require.NotNil(t, cloned.Lyrics)
	require.NotNil(t, cloned.Seed)
	require.NotSame(t, src.Lyrics, cloned.Lyrics)
	require.NotSame(t, src.Seed, cloned.Seed)

	src.Lyrics.Keywords[0] = "changed"
	*src.Seed = 99

	assert.Equal(t, "one", cloned.Lyrics.Keywords[0])
	assert.Equal(t, int64(7), *cloned.Seed)
	assert.Equal(t, "A minor", cloned.Key)
	assert.Equal(t, "Japanese female vocal, clear diction", cloned.VocalProfile)
	if assert.Len(t, cloned.Sections, 1) {
		assert.Equal(t, 10, cloned.Sections[0].StartSeconds)
		assert.Equal(t, 40, cloned.Sections[0].EndSeconds)
	}
}

func TestLyriaAudioGeneratorSingleflightDeduplicatesConcurrentCalls(t *testing.T) {
	ctx := context.Background()
	client := newBlockingGeminiClient()
	client.partsResp = &gemini.Response{Audios: [][]byte{{1, 2, 3}}}
	seed := int64(7)

	generator := &lyriaAudioGenerator{
		aiClient:          client,
		defaultLyriaModel: "lyria-3",
		limiter:           rate.NewLimiter(rate.Inf, 0),
		promptBuilder:     fixedAudioPromptBuilder{fullSong: "full prompt"},
		converter:         noopPhoneticConverter{},
	}

	recipe := &MusicRecipe{
		Title:       "Song",
		Mood:        "Bright",
		Tempo:       140,
		Instruments: []string{"synth"},
		Sections: []MusicSection{
			{Name: "Verse", Duration: 30, Prompt: "pulse"},
		},
		AIModels: AIModels{Seed: &seed},
	}

	images := []ImagePayload{
		{Data: []byte("fake-image"), MIMEType: "image/png"},
	}

	const callers = 5
	results := make([][]byte, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := range callers {
		go func(i int) {
			defer wg.Done()
			// 修正：引数に images を追加
			results[i], errs[i] = generator.GenerateAudio(ctx, recipe, images)
		}(i)
	}

	require.Eventually(t, func() bool {
		select {
		case <-client.partsStartedCh:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)

	time.Sleep(20 * time.Millisecond)
	close(client.releasePartsCh)
	wg.Wait()

	// singleflight により、1回しか API が呼ばれていないことを確認
	require.Equal(t, int32(1), client.partsCalls.Load())
	for _, err := range errs {
		require.NoError(t, err)
	}

	// 修正後の cloneBytes の検証：バイナリが独立したメモリ領域であることを確認
	results[0][0] = 9
	assert.Equal(t, byte(1), results[1][0])
}

func TestLyriaAudioGeneratorSingleflightSeparatesDifferentImages(t *testing.T) {
	ctx := context.Background()
	client := newBlockingGeminiClient()
	client.partsResp = &gemini.Response{Audios: [][]byte{{1, 2, 3}}}
	seed := int64(7)

	generator := &lyriaAudioGenerator{
		aiClient:          client,
		defaultLyriaModel: "lyria-3",
		limiter:           rate.NewLimiter(rate.Inf, 0),
		promptBuilder:     fixedAudioPromptBuilder{fullSong: "full prompt"},
		converter:         noopPhoneticConverter{},
	}

	recipe := &MusicRecipe{
		Title:       "Song",
		Mood:        "Bright",
		Tempo:       140,
		Instruments: []string{"synth"},
		Sections: []MusicSection{
			{Name: "Verse", Duration: 30, Prompt: "pulse"},
		},
		AIModels: AIModels{Seed: &seed},
	}

	imagesA := []ImagePayload{{Data: []byte("image-a"), MIMEType: "image/png"}}
	imagesB := []ImagePayload{{Data: []byte("image-b"), MIMEType: "image/png"}}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = generator.GenerateAudio(ctx, recipe, imagesA)
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = generator.GenerateAudio(ctx, recipe, imagesB)
	}()

	require.Eventually(t, func() bool {
		return client.partsCalls.Load() == 2
	}, time.Second, time.Millisecond)

	close(client.releasePartsCh)
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
}
