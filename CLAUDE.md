# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Go library wrapping the official `google.golang.org/genai` SDK for Gemini API / Vertex AI, plus generation workflows built on top of it. Three packages, no main:

- `gemini/` — retrying client (text/multimodal generation, File API upload with Active-state polling, response extraction into `Response{Text, Images, Audios, Attachments}`, and the one-round-trip Veo video surface `StartVideo`/`PollVideo`)
- `lyria/` — lyrics → recipe → audio music generation using `gemini.MultimodalGenerator`, exposed as three independently callable steps
- `veo/` — Veo video generation: submits the long-running operation and owns *how to wait* for it, using an injected `gemini.VideoGenerator`

**This module is the only place in the workspace that imports `google.golang.org/genai`.** Downstream repos depend on it precisely so the SDK stays confined here; adding a direct genai call anywhere else defeats that. When a Google AI API is not yet wrapped, add it here rather than hand-rolling REST in the app.

## Commands

```sh
go build ./...
go vet ./...
go test -race ./...                          # CI runs tests with -race
go test ./gemini/ -run TestShouldRetry       # single test
test -z "$(gofmt -l .)"                      # CI fails on unformatted code
golangci-lint run                            # CI uses v2.12.2; config in .golangci.yml
```

Tests requiring GCP Application Default Credentials (Vertex AI client construction) skip themselves automatically via `skipWithoutGCPCredentials`.

## Architecture

### gemini package

- `Client` wraps the genai SDK behind two small internal interfaces (`modelClient`, `fileClient` in `sdk.go`); tests substitute fakes (`fakeModelClient`, `fakeFileClient`) instead of hitting the network.
- **Retry**: every generate call goes through the `runWithRetry` generic helper in `client.go`, which wraps `retry.RunValue` (from `github.com/shouni/netarmor/retry`) with the `shouldRetry` predicate in `helpers.go`. Options come from `Config.buildRetryOptions()`; the inspectable `retryParams` intermediate exists so tests can assert which values were resolved (`retry.Option` values are opaque funcs). Note `retry.WithMaxRetries(0)` means "no retry", so `retryParams` falls back to `DefaultMaxRetries` when `Config.MaxRetries` is 0. The genai SDK communicates over **REST, not gRPC** — API errors are `genai.APIError` **values** carrying HTTP status codes. Retry on 429/500/503/504; `APIResponseError` (safety blocks, empty responses) and context cancellation never retry. `Config.OnRetry` maps to `retry.WithNotify` for observability.
- **Config**: `APIKey` (Gemini API) and `ProjectID`+`LocationID` (Vertex AI) are mutually exclusive; validation and backend selection live in `config.go`. Defaults (retry, file polling) are resolved once in `NewClient` — Client fields are always populated, no fallback at use sites.
- **File API**: `UploadFile` returns an `UploadedFile{URI, Name}` struct — the two are both strings with different uses (`URI` goes into `genai.FileData`, `Name` into `DeleteFile`), so they are not returned as bare strings. It polls until the file is Active; on failure after a successful upload it fires `asyncDelete` (detached, fire-and-forget — there is no way to await it).
- Public consumer-facing interfaces (`Generator`, `FileManager`, …) are in `interfaces.go`; `lyria` and downstream apps mock against these.
- **Two entry points for multimodal input, and the difference is the SDK leak.** `GenerateWithParts` takes `[]*genai.Part`, so every caller *and every test mock* has to import genai. `GenerateWithAttachments` takes a prompt plus `[]Attachment` (`MIMEType` + either `Data` or `URI`) and builds the parts internally, which is what downstream repos should use — `MultimodalGenerator` is a single method and mocks in one line. Keep `GenerateWithParts` for callers that genuinely need Part-level control (interleaved ordering, per-part system instructions). `attachmentParts` in `attachment.go` is the one place that converts; it drops empty attachments (callers assemble optional images as "pass it if we have it"), rejects `Data`+`URI` together, and requires a MIME type only for inline data since a URI's type can be left to the server.
- **`Response.Attachments` exists so callers don't have to read `RawResponse`.** `Images`/`Audios` are bytes only, so deciding a file extension or Content-Type meant walking the genai response. `Attachments` carries the MIME type alongside the bytes, in return order.
- **`SafetyThreshold` and `ThinkingLevel` are aliases of the genai types**, with `SafetyBlockNone`/`ThinkingMinimal`/… constants re-exported. This keeps `NewSafetySettings`'s signature unchanged while letting callers pick a value without importing genai. Vertex AI rejects `SafetyOff`; use `SafetyBlockNone` there.
- **Errors** live in `errors.go`. `APIResponseError` carries a `Reason` sentinel (`ErrBlocked` / `ErrEmptyResponse`) plus the `FinishReason`, and `Unwrap` returns the sentinel so `errors.Is` classifies it. Input-validation sentinels (`ErrEmptyPrompt`, …) are Japanese and part of the public API; the newer response sentinels are English.
- **`genai.FinishReason` has two "unset" values.** Its Go zero value is `""`, but the SDK constant `FinishReasonUnspecified` is the string `"FINISH_REASON_UNSPECIFIED"`. Comparing only against the constant misclassifies streaming chunks (which carry no finish reason) as blocked. Always go through `isUnsetFinishReason` / `isBlockedFinishReason`.
- **`extractText` concatenates all non-`Thought` text parts.** Thinking-enabled models return the thought summary as a `Thought: true` part *before* the answer, and long answers can be split across parts — returning the first non-empty part gives you the wrong text. Thought parts are surfaced separately as `Response.Thoughts` via `extractThoughts`.
- **`GenerateOptions` uses pointers where zero is meaningful** (`Temperature`, `TopP`, `TopK`, `ThinkingBudget`) — `Temperature: 0` means deterministic, not unset. Use the `Ptr` helper. `ThinkingConfig` is built by `buildThinkingConfig` and is only sent when something is actually specified, so the model's default thinking behavior isn't silently overridden. `ThinkingLevel` (model-independent tiers) and `ThinkingBudget` (token count) are alternative ways to say the same thing — when both are set, `ThinkingLevel` wins and the budget is dropped rather than sending a combination whose precedence is undefined. `ResponseJSONSchema` vs `ResponseSchema` follows the same rule.
- **Neither of those is deprecated**, despite appearances: the SDK's `GenerationConfig` (the batch/tuning payload type produced by `ToGenerationConfig`) marks its `ResponseSchema`/`ResponseMIMEType` `Deprecated: Use response_format instead`, but `GenerateContentConfig` — the type this package actually sends — has zero deprecation markers. Don't "fix" our usage based on the wrong struct.
- `Config.HTTPClient` is passed to `genai.ClientConfig`, so a netarmor `securenet.NewSafeHTTPClient` can be injected.

### veo package

- **The split between `gemini` and `veo` is "one round trip" vs "how to wait".** `gemini.StartVideo`/`PollVideo` own the genai client and do exactly one call each; `veo.Client` owns the polling interval, the overall timeout, and how many consecutive poll failures to absorb. Video generation is the only long-running-operation API here, so this is the only place that distinction exists.
- **`PollVideo` deliberately does *not* go through `runWithRetry`, while `StartVideo` does.** Polling is already a retry loop; nesting `retry`'s backoff (default 30s initial / 120s max) inside it makes a single poll take tens of seconds and silently invalidates the caller's interval and timeout. Submission is the opposite case — a 429 there loses a whole video, so it must be retried. Don't "fix" the asymmetry.
- `veo` never imports genai. Its `Request`/`Reference`/`Media` are aliases of `gemini.VideoRequest`/`VideoReference`/`Attachment`, so there is no parallel type set to keep in sync, and `gemini.VideoGenerator` is two methods — tests use a fake with no SDK and no GCP credentials.
- **Input combinations are validated before sending** in `VideoRequest.buildSource`: Veo cannot combine `video` with `image`/`referenceImages`, and `lastFrame` is only valid alongside `image`. These are API-level facts, not policy — the *choice* of which mode to use belongs to the caller (`go-veo-orchestrator/ports.ClassifyVeoRequest` is where that decision lives for the MV pipeline).
- **`Request.ExtraBody` is the escape hatch for a missing *field*, not a missing *endpoint*** (it maps to `genai.HTTPOptions.ExtraBody`). It adds keys to the body of a call the SDK already makes, so it covers preview parameters on `generateContent`/`generateVideos` — and nothing else. When the SDK has no client for the endpoint at all (as of genai v1.66.0: the Interactions API behind `gemini-omni-flash-preview`), ExtraBody is useless and the choice is to wait for SDK support or write the call here. Nothing in ExtraBody is type-checked, so anything the SDK *does* model must use the real field.
- **ExtraBody cannot reach inside an array, and failing that quietly is destructive.** genai merges it with `recursiveMapMerge`, which recurses only when both sides are `map[string]any`; for any other type it overwrites. Vertex bodies are `{"instances": [...], "parameters": {...}}`, so `ExtraBody: {"instances": [...]}` replaces the whole array and takes the prompt and image inputs with it. `Request.ModifyRequestBody` (genai's `ExtrasRequestProvider`) receives the assembled body after that merge and is the only way to add a key inside `instances[0]`.
- Operation-level failure is reported on `VideoOperation.Failure` (wrapping `ErrVideoGenerationFailed`), not as the method's error: fetching the operation succeeded, so a transport error would misclassify it. genai exposes the failure as an untyped `map[string]any` (google.rpc.Status), so `videoOperationFailure` picks out code/status/message.

### lyria package

- `Workflow` is a facade over three roles: `Lyricist`/`Composer` (both implemented by `lyriaTextGenerator`) and `AudioGenerator` (`lyriaAudioGenerator`). Prompt construction is injected by the caller via `TextPromptGenerator` and `AudioPromptBuilder` — this library contains no prompt text.
- **There is deliberately no all-in-one `Run`.** One existed and no caller used it: ap-comp calls the three steps itself so it can gate between them (verify the composed recipe against the compose mode's declared structure and recompose with a shifted seed on failure, then measure and optionally review the audio). Those gates are per-product, so a bundled entry point only gets decomposed again at the call site. If a caller wants the straight-line sequence, it is `GenerateLyrics` → `Compose` → `GenerateAudio` in that order.
- `Compose` attaches `AIModels` and `Seed` to the recipe *after* generation — the recipe schema omits them on purpose so the model never invents them. `TestComposeAttachesModelsAndSeed` guards this; without it a seeded, reproducible run silently stops being reproducible.
- Text generation (lyrics and recipe) shares one generic pipeline: `generateJSON[T]` in `text.go` (singleflight → Gemini call with JSON MIME type + `ResponseSchema` from `schemas.go` → `cleanJSONResponse` → unmarshal). The recipe schema deliberately omits `lyrics`/model fields — code attaches those after generation.
- **Singleflight + clone pattern**: identical concurrent requests are deduplicated via `doSingleflight` (`singleflight.go`), which detaches from the caller's context (`context.WithoutCancel` + a per-run `execTimeout`, settable via `WithExecTimeout`, default 5m). Because results are shared across callers, every public method must return a **clone** (`cloneLyricsDraft`, `cloneMusicRecipe`, `cloneBytes`) and must not write caller-specific data into the shared result.
- Per-call model/mode/seed selection comes from the `AIModels` argument, falling back to the models set via `New(...)` options.
- **Everything that changes the output must be in the singleflight key.** `seed` in particular: it is passed to the generate call but is *not* implied by the prompt, so omitting it makes concurrent different-seed calls share one result. Use `singleflightSeedKey`. Both `generateJSON` and `GenerateAudio` do this.
- `AIModels` is embedded in `MusicRecipe`, so its fields are flattened into recipe JSON — it carries explicit snake_case tags to match the rest of the struct.
- Audio generation is rate-limited (`WithRateInterval`) and passes `Seed` through unconditionally (no backend-specific special-casing).

## Conventions

- Comments and error messages are largely Japanese; match the surrounding file.
- Go 1.26 idioms are used (`errors.AsType`, `new(expr)`).
- Update README.md when public API (Config fields, GenerateOptions, sentinel errors, interfaces) changes — it documents them in tables. Sample code names concrete models; refresh them when the current generation moves on.
- `CleanJSONResponse` is fuzzed (`FuzzCleanJSONResponse`, run for 60s in CI). Its invariant: if it changes the input at all, the result must be valid JSON — otherwise it leaves callers worse off than the raw string.
