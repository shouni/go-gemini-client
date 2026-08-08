package gemini

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

func TestAttachmentPartsBuildsTextThenInlineData(t *testing.T) {
	parts, err := attachmentParts("describe this", []Attachment{
		{MIMEType: "audio/mpeg", Data: []byte("song")},
		{MIMEType: "image/png", Data: []byte("cover")},
	})
	if err != nil {
		t.Fatalf("attachmentParts() error = %v", err)
	}

	if len(parts) != 3 {
		t.Fatalf("parts = %d, want 3", len(parts))
	}
	if parts[0].Text != "describe this" {
		t.Errorf("parts[0].Text = %q, want the prompt first", parts[0].Text)
	}
	for i, want := range []Attachment{
		{MIMEType: "audio/mpeg", Data: []byte("song")},
		{MIMEType: "image/png", Data: []byte("cover")},
	} {
		blob := parts[i+1].InlineData
		if blob == nil {
			t.Fatalf("parts[%d].InlineData is nil", i+1)
		}
		if blob.MIMEType != want.MIMEType || string(blob.Data) != string(want.Data) {
			t.Errorf("parts[%d] = %q/%q, want %q/%q", i+1, blob.MIMEType, blob.Data, want.MIMEType, want.Data)
		}
	}
}

// TestAttachmentPartsSkipsEmptyData verifies empty attachments drop out. Callers assemble optional
// images as "pass it if we have it", and making each of them filter empties is busywork.
func TestAttachmentPartsSkipsEmptyData(t *testing.T) {
	parts, err := attachmentParts("prompt", []Attachment{
		{MIMEType: "image/png"},
		{MIMEType: "image/png", Data: []byte("real")},
	})
	if err != nil {
		t.Fatalf("attachmentParts() error = %v", err)
	}

	if len(parts) != 2 {
		t.Fatalf("parts = %d, want 2 (the empty attachment should be dropped)", len(parts))
	}
}

// TestAttachmentPartsAllowsAttachmentOnlyRequests verifies a request may carry no prompt. Handing
// the model an audio file and asking for structured output via the schema alone is a valid shape.
func TestAttachmentPartsAllowsAttachmentOnlyRequests(t *testing.T) {
	parts, err := attachmentParts("", []Attachment{{MIMEType: "audio/mpeg", Data: []byte("song")}})
	if err != nil {
		t.Fatalf("attachmentParts() error = %v", err)
	}

	if len(parts) != 1 || parts[0].InlineData == nil {
		t.Fatalf("parts = %+v, want a single inline-data part", parts)
	}
}

// TestAttachmentPartsRejectsEmptyRequests verifies nothing-to-send is an error rather than a request
// with an empty content list, which the API rejects with a less obvious message.
func TestAttachmentPartsRejectsEmptyRequests(t *testing.T) {
	tests := map[string][]Attachment{
		"no prompt and no attachments": nil,
		"only empty attachments":       {{MIMEType: "image/png"}},
	}

	for name, attachments := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := attachmentParts("", attachments); !errors.Is(err, ErrEmptyParts) {
				t.Errorf("attachmentParts() error = %v, want ErrEmptyParts", err)
			}
		})
	}
}

// TestAttachmentPartsRejectsMissingMIMEType verifies data without a MIME type fails loudly. The API
// cannot infer it, and a silent send produces a confusing model-side error instead.
func TestAttachmentPartsRejectsMissingMIMEType(t *testing.T) {
	_, err := attachmentParts("prompt", []Attachment{{Data: []byte("bytes")}})

	if err == nil {
		t.Fatal("attachmentParts() error = nil, want an error")
	}
	if errors.Is(err, ErrEmptyParts) {
		t.Errorf("attachmentParts() error = %v, want a MIME type error", err)
	}
}

func TestGenerateWithAttachmentsSendsPromptAndInlineData(t *testing.T) {
	fake := &fakeModelClient{}
	client := &Client{modelClient: fake, retryOpts: Config{MaxRetries: 1}.buildRetryOptions()}

	resp, err := client.GenerateWithAttachments(context.Background(), "gemini-test", "review this",
		[]Attachment{{MIMEType: "audio/mpeg", Data: []byte("song")}}, GenerateOptions{})
	if err != nil {
		t.Fatalf("GenerateWithAttachments() error = %v", err)
	}
	if resp.Text != "ok" {
		t.Errorf("resp.Text = %q, want %q", resp.Text, "ok")
	}

	if len(fake.gotContents) != 1 {
		t.Fatalf("contents = %d, want 1", len(fake.gotContents))
	}
	parts := fake.gotContents[0].Parts
	if len(parts) != 2 {
		t.Fatalf("parts = %d, want 2", len(parts))
	}
	if parts[0].Text != "review this" {
		t.Errorf("parts[0].Text = %q", parts[0].Text)
	}
	if parts[1].InlineData == nil || parts[1].InlineData.MIMEType != "audio/mpeg" {
		t.Errorf("parts[1] = %+v, want the audio attachment", parts[1])
	}
}

// TestGenerateWithAttachmentsAppliesGenerateOptions verifies the options path is shared with
// GenerateWithParts, so structured output works the same through either entry point.
func TestGenerateWithAttachmentsAppliesGenerateOptions(t *testing.T) {
	fake := &fakeModelClient{}
	client := &Client{modelClient: fake, retryOpts: Config{MaxRetries: 1}.buildRetryOptions()}

	schema := map[string]any{"type": "object"}
	_, err := client.GenerateWithAttachments(context.Background(), "gemini-test", "prompt",
		[]Attachment{{MIMEType: "audio/mpeg", Data: []byte("song")}},
		GenerateOptions{ResponseMIMEType: "application/json", ResponseJSONSchema: schema})
	if err != nil {
		t.Fatalf("GenerateWithAttachments() error = %v", err)
	}

	if fake.gotConfig.ResponseMIMEType != "application/json" {
		t.Errorf("ResponseMIMEType = %q", fake.gotConfig.ResponseMIMEType)
	}
	if fake.gotConfig.ResponseJsonSchema == nil {
		t.Error("ResponseJsonSchema was not forwarded")
	}
}

// TestGenerateWithAttachmentsValidatesModelName verifies the shared validation still runs; the
// attachment entry point must not become a way to skip it.
func TestGenerateWithAttachmentsValidatesModelName(t *testing.T) {
	client := &Client{modelClient: &fakeModelClient{}, retryOpts: Config{MaxRetries: 1}.buildRetryOptions()}

	_, err := client.GenerateWithAttachments(context.Background(), "", "prompt",
		[]Attachment{{MIMEType: "audio/mpeg", Data: []byte("song")}}, GenerateOptions{})

	if !errors.Is(err, ErrEmptyModelName) {
		t.Errorf("GenerateWithAttachments() error = %v, want ErrEmptyModelName", err)
	}
}

// TestAttachmentMatchesGenaiBlobShape guards the assumption behind the type: an Attachment carries
// exactly what an inline genai.Blob needs, so no information is lost by using it instead.
func TestAttachmentMatchesGenaiBlobShape(t *testing.T) {
	attachment := Attachment{MIMEType: "audio/mpeg", Data: []byte("song")}
	blob := genai.Blob{MIMEType: attachment.MIMEType, Data: attachment.Data}

	if blob.MIMEType != attachment.MIMEType || string(blob.Data) != string(attachment.Data) {
		t.Error("Attachment no longer maps cleanly onto genai.Blob")
	}
}

// TestAttachmentPartsBuildsFileDataForURIs verifies a URI reference becomes a FileData part rather
// than inline bytes. Vertex AI can read gs:// directly and the File API hands back a URI, so both
// paths need to be expressible without the caller touching genai.
func TestAttachmentPartsBuildsFileDataForURIs(t *testing.T) {
	parts, err := attachmentParts("describe", []Attachment{
		{URI: "gs://bucket/cover.png", MIMEType: "image/png"},
		{URI: "files/abc123"},
	})
	if err != nil {
		t.Fatalf("attachmentParts() error = %v", err)
	}

	if len(parts) != 3 {
		t.Fatalf("parts = %d, want 3", len(parts))
	}
	if parts[1].FileData == nil || parts[1].FileData.FileURI != "gs://bucket/cover.png" {
		t.Errorf("parts[1] = %+v, want the gs:// reference", parts[1])
	}
	if parts[1].FileData.MIMEType != "image/png" {
		t.Errorf("parts[1] MIME type = %q, want it forwarded", parts[1].FileData.MIMEType)
	}
	// MIME type を推測できない URI は、サーバー側の判定に委ねられるよう空のまま送る。
	if parts[2].FileData == nil || parts[2].FileData.MIMEType != "" {
		t.Errorf("parts[2] = %+v, want an empty MIME type", parts[2])
	}
	if parts[1].InlineData != nil || parts[2].InlineData != nil {
		t.Error("URI attachments must not be sent as inline data")
	}
}

// TestAttachmentPartsRejectsDataAndURITogether verifies the two are exclusive. Sending both leaves
// it to the API which one wins, and the caller almost certainly meant only one.
func TestAttachmentPartsRejectsDataAndURITogether(t *testing.T) {
	_, err := attachmentParts("prompt", []Attachment{
		{MIMEType: "image/png", Data: []byte("bytes"), URI: "gs://bucket/cover.png"},
	})

	if err == nil {
		t.Fatal("attachmentParts() error = nil, want an error")
	}
}

// TestAttachmentIsEmptyCoversBothCarriers verifies the skip rule sees URI-only attachments as
// present. Treating them as empty would silently drop every reference-based image.
func TestAttachmentIsEmptyCoversBothCarriers(t *testing.T) {
	tests := map[string]struct {
		attachment Attachment
		want       bool
	}{
		"nothing set": {attachment: Attachment{MIMEType: "image/png"}, want: true},
		"data set":    {attachment: Attachment{Data: []byte("x")}, want: false},
		"uri set":     {attachment: Attachment{URI: "files/abc"}, want: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tt.attachment.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestResponseAttachmentsCarryMIMETypes verifies returned inline data keeps its MIME type. Images
// and Audios are bytes only, so without this the caller cannot know what to name the file it saves.
func TestResponseAttachmentsCarryMIMETypes(t *testing.T) {
	raw := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			FinishReason: genai.FinishReasonStop,
			Content: &genai.Content{Parts: []*genai.Part{
				{InlineData: &genai.Blob{MIMEType: "image/png", Data: []byte("png")}},
				{InlineData: &genai.Blob{MIMEType: "audio/mpeg", Data: []byte("mp3")}},
			}},
		}},
	}

	resp, err := responseFromGenAI(raw)
	if err != nil {
		t.Fatalf("responseFromGenAI() error = %v", err)
	}

	if len(resp.Attachments) != 2 {
		t.Fatalf("attachments = %d, want 2", len(resp.Attachments))
	}
	if resp.Attachments[0].MIMEType != "image/png" || string(resp.Attachments[0].Data) != "png" {
		t.Errorf("attachments[0] = %+v", resp.Attachments[0])
	}
	if resp.Attachments[1].MIMEType != "audio/mpeg" {
		t.Errorf("attachments[1] MIME type = %q", resp.Attachments[1].MIMEType)
	}
	// 既存の Images / Audios は従来どおり振り分けられていること。
	if len(resp.Images) != 1 || len(resp.Audios) != 1 {
		t.Errorf("images = %d, audios = %d, want 1 each", len(resp.Images), len(resp.Audios))
	}
}
