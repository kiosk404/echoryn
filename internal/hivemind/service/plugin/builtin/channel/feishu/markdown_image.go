package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// ImageTransformer processes Markdown image links for Feishu rendering.
//
// Feishu card/post messages require images to use Feishu's internal image keys
// (format: img_v2_xxx or img_xxx), not external URLs.
//
// This transformer:
//  1. Detects ![alt](url) patterns in markdown.
//  2. For external URLs (http/https): uploads the image to Feishu and replaces with img_key.
//  3. For already-resolved keys (img_xxx): keeps them as-is.
//  4. For invalid references: removes them to prevent rendering errors.
//
// Architecture:
//   - Uses an internal cache to avoid re-uploading the same URL.
//   - Upload is synchronous with a timeout to avoid blocking the pipeline.
//   - Thread-safe via sync.RWMutex on the cache.
//
// Reference: openclaw-lark's ImageResolver class.
type ImageTransformer struct {
	// getToken is a function that returns a valid tenant access token.
	getToken func(ctx context.Context) (string, error)

	// domain specifies feishu or lark API endpoints.
	domain DomainType

	// cache stores URL → img_key mappings to avoid re-uploads.
	mu    sync.RWMutex
	cache map[string]string

	// failed stores URLs that have failed upload, to avoid retrying.
	failed map[string]struct{}

	// uploadTimeout is the maximum time allowed for a single image upload.
	uploadTimeout time.Duration
}

// NewImageTransformer creates a new ImageTransformer.
func NewImageTransformer(getToken func(ctx context.Context) (string, error), domain DomainType) *ImageTransformer {
	return &ImageTransformer{
		getToken:      getToken,
		domain:        domain,
		cache:         make(map[string]string),
		failed:        make(map[string]struct{}),
		uploadTimeout: 15 * time.Second,
	}
}

const (
	feishuImageUploadURL = "https://open.feishu.cn/open-apis/im/v1/images"
	larkImageUploadURL   = "https://open.larksuite.com/open-apis/im/v1/images"
)

// imageMarkdownRegex matches ![alt](value) patterns.
var imageMarkdownRegex = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

func (t *ImageTransformer) Transform(text string, mode RenderMode) string {
	if mode != RenderModeCard {
		// Post mode: remove image links entirely (post doesn't support inline images well).
		return imageMarkdownRegex.ReplaceAllString(text, "")
	}

	return imageMarkdownRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := imageMarkdownRegex.FindStringSubmatch(match)
		if len(submatch) < 3 {
			return ""
		}
		alt := submatch[1]
		value := submatch[2]

		// Already a Feishu image key — keep as-is.
		if strings.HasPrefix(value, "img_") {
			return match
		}

		// Not a URL — remove invalid reference.
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			return ""
		}

		// Check cache first.
		t.mu.RLock()
		if imgKey, ok := t.cache[value]; ok {
			t.mu.RUnlock()
			return fmt.Sprintf("![%s](%s)", alt, imgKey)
		}
		if _, failed := t.failed[value]; failed {
			t.mu.RUnlock()
			return "" // Previously failed, don't retry.
		}
		t.mu.RUnlock()

		// Attempt synchronous upload.
		imgKey, err := t.uploadImage(value)
		if err != nil {
			logger.Warn("[Feishu/Image] upload failed for %s: %v", value, err)
			t.mu.Lock()
			t.failed[value] = struct{}{}
			t.mu.Unlock()
			return "" // Remove failed image reference.
		}

		// Cache successful upload.
		t.mu.Lock()
		t.cache[value] = imgKey
		t.mu.Unlock()

		return fmt.Sprintf("![%s](%s)", alt, imgKey)
	})
}

// uploadImage downloads an image from the URL and uploads it to Feishu.
// Returns the Feishu image key (img_xxx format).
//
// API: POST https://open.feishu.cn/open-apis/im/v1/images
// Content-Type: multipart/form-data
// Fields: image_type=message, image=<binary>
func (t *ImageTransformer) uploadImage(imageURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), t.uploadTimeout)
	defer cancel()

	// Step 1: Download the image.
	imgReq, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("create download request: %w", err)
	}

	imgResp, err := http.DefaultClient.Do(imgReq)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer imgResp.Body.Close()

	if imgResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download image: status %d", imgResp.StatusCode)
	}

	// Read image data (limit to 20MB to match Feishu's limit).
	const maxImageSize = 20 * 1024 * 1024
	imgData, err := io.ReadAll(io.LimitReader(imgResp.Body, maxImageSize))
	if err != nil {
		return "", fmt.Errorf("read image data: %w", err)
	}

	// Step 2: Upload to Feishu.
	token, err := t.getToken(ctx)
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}

	// Build multipart form.
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if err := writer.WriteField("image_type", "message"); err != nil {
		return "", fmt.Errorf("write image_type field: %w", err)
	}

	part, err := writer.CreateFormFile("image", "image.png")
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(imgData); err != nil {
		return "", fmt.Errorf("write image data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	uploadURL := feishuImageUploadURL
	if t.domain == DomainLark {
		uploadURL = larkImageUploadURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &buf)
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload image: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ImageKey string `json:"image_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse upload response: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu API error: code=%d, msg=%s", result.Code, result.Msg)
	}

	logger.Debug("[Feishu/Image] uploaded %s → %s", imageURL, result.Data.ImageKey)
	return result.Data.ImageKey, nil
}

// StripInvalidImages removes image references that don't have valid Feishu image keys.
// This acts as a safety net after all processing to prevent CardKit error 200570.
//
// Reference: openclaw-lark's stripInvalidImageKeys function.
type StripInvalidImages struct{}

func (s *StripInvalidImages) Transform(text string, mode RenderMode) string {
	if mode != RenderModeCard {
		return text
	}

	return imageMarkdownRegex.ReplaceAllStringFunc(text, func(match string) string {
		submatch := imageMarkdownRegex.FindStringSubmatch(match)
		if len(submatch) < 3 {
			return ""
		}
		value := submatch[2]
		if strings.HasPrefix(value, "img_") {
			return match // Valid Feishu image key — keep.
		}
		return "" // Invalid — remove.
	})
}
