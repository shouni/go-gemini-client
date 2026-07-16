# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Go library wrapping the official `google.golang.org/genai` SDK for Gemini API / Vertex AI, plus a music-generation workflow built on top of it. Two packages, no main:

- `gemini/` — retrying client (text/multimodal generation, File API upload with Active-state polling, response extraction into `Response{Text, Images, Audios}`)
- `lyria/` — lyrics → recipe → audio music-generation workflow facade using `gemini.Generator`

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
- **Retry**: every generate call goes through `retry.Do` (from `github.com/shouni/netarmor/retry`) with the `shouldRetry` predicate in `helpers.go`. The genai SDK communicates over **REST, not gRPC** — API errors are `genai.APIError` **values** carrying HTTP status codes. Retry on 429/500/503/504; `APIResponseError` (safety blocks, empty responses) and context cancellation never retry.
- **Config**: `APIKey` (Gemini API) and `ProjectID`+`LocationID` (Vertex AI) are mutually exclusive; validation and backend selection live in `config.go`. Defaults (retry, file polling) are resolved once in `NewClient` — Client fields are always populated, no fallback at use sites.
- **File API**: `UploadFile` polls until the file is Active; on failure after a successful upload it fires `asyncDelete` (detached context) to clean up server-side.
- Public consumer-facing interfaces (`Generator`, `FileManager`, …) are in `interfaces.go`; `lyria` and downstream apps mock against these.
- Error messages and sentinel errors in this package are in Japanese; exported sentinels (`ErrEmptyPrompt`, etc.) are part of the public API and documented in README.

### lyria package

- `Workflow` is a facade over three roles: `Lyricist`/`Composer` (both implemented by `lyriaTextGenerator`) and `AudioGenerator` (`lyriaAudioGenerator`). Prompt construction is injected by the caller via `TextPromptGenerator` and `AudioPromptBuilder` — this library contains no prompt text.
- Text generation (lyrics and recipe) shares one generic pipeline: `generateJSON[T]` in `text.go` (singleflight → Gemini call with JSON MIME type → `cleanJSONResponse` → unmarshal).
- **Singleflight + clone pattern**: identical concurrent requests are deduplicated via `doSingleflight` (`singleflight.go`), which detaches from the caller's context (`context.WithoutCancel` + `singleflightExecTimeout`). Because results are shared across callers, every public method must return a **clone** (`cloneLyricsDraft`, `cloneMusicRecipe`, `cloneBytes`) and must not write caller-specific data into the shared result.
- Per-call model/mode/seed selection comes from the `AIModels` argument, falling back to the models set via `New(...)` options.
- Audio generation is rate-limited (`WithRateInterval`) and passes `Seed` through unconditionally (no backend-specific special-casing).

## Conventions

- Comments and error messages are largely Japanese; match the surrounding file.
- Go 1.26 idioms are used (`errors.AsType`, `new(expr)`).
- Update README.md when public API (Config fields, GenerateOptions, sentinel errors) changes — it documents them in tables.
