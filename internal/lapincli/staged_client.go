package lapincli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const (
	stagedChapterBatchBytes = maxRequestBytes - (64 << 10)
	stagedAssetBatchSize    = 16
	stagedAssetBatchBytes   = 10 << 20
)

type stagedChapter struct {
	ExternalID       string  `json:"external_id"`
	ParentExternalID *string `json:"parent_external_id"`
	Position         int     `json:"position"`
	Title            string  `json:"title"`
	Content          string  `json:"content"`
}

func importStagedBundle(ctx context.Context, baseURL, token string, bundle loadedBundle, dependencyClient *http.Client) (importResult, error) {
	chapters := flattenImportChapters(bundle.Request.Chapters)
	idempotencyKey, err := bundleDigest(bundle)
	if err != nil {
		return importResult{}, err
	}
	var started struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		SubjectID string `json:"subject_id"`
	}
	err = stagedJSONRequest(ctx, baseURL, "/openapi/v1/subject-imports", token, map[string]any{
		"idempotency_key":   idempotencyKey,
		"external_id":       bundle.Request.ExternalID,
		"title":             bundle.Request.Title,
		"description":       bundle.Request.Description,
		"tags":              bundle.Request.Tags,
		"expected_chapters": len(chapters),
		"expected_assets":   len(bundle.Assets),
	}, dependencyClient, &started)
	if err != nil {
		return importResult{}, err
	}
	if !validSubjectID(started.ID) {
		return importResult{}, fmt.Errorf("staged import response does not contain a valid import ID")
	}
	if started.Status == "committed" && validSubjectID(started.SubjectID) {
		return importResult{SubjectID: started.SubjectID}, nil
	}
	if started.Status != "draft" {
		return importResult{}, fmt.Errorf("staged import is not writable")
	}

	assetURLs := make(map[string]string, len(bundle.Assets))
	for _, batch := range partitionStagedAssets(bundle.Assets) {
		var uploaded struct {
			Assets []struct {
				Key string `json:"key"`
				URL string `json:"url"`
			} `json:"assets"`
		}
		path := "/openapi/v1/subject-imports/" + started.ID + "/assets"
		if err := stagedAssetRequest(ctx, baseURL, path, token, batch, dependencyClient, &uploaded); err != nil {
			return importResult{}, err
		}
		expectedKeys := make(map[string]struct{}, len(batch))
		for _, asset := range batch {
			expectedKeys[asset.Key] = struct{}{}
		}
		if len(uploaded.Assets) != len(expectedKeys) {
			return importResult{}, fmt.Errorf("asset upload response does not match the request")
		}
		for _, asset := range uploaded.Assets {
			if _, exists := expectedKeys[asset.Key]; !exists {
				return importResult{}, fmt.Errorf("asset upload response contains an unknown key")
			}
			if _, duplicate := assetURLs[asset.Key]; duplicate || !strings.HasPrefix(asset.URL, "/api/v1/assets/") || !strings.HasSuffix(asset.URL, "/content") {
				return importResult{}, fmt.Errorf("asset upload response contains an invalid URL")
			}
			assetURLs[asset.Key] = asset.URL
		}
	}
	for index := range chapters {
		for key, assetURL := range assetURLs {
			chapters[index].Content = strings.ReplaceAll(chapters[index].Content, "lapin-asset://"+key, assetURL)
		}
		if strings.Contains(chapters[index].Content, "lapin-asset://") {
			return importResult{}, fmt.Errorf("chapter %q references an unknown local asset", chapters[index].ExternalID)
		}
	}

	batches, err := partitionStagedChapters(chapters)
	if err != nil {
		return importResult{}, err
	}
	for index, batch := range batches {
		encoded, _ := json.Marshal(batch)
		digest := sha256.Sum256(encoded)
		batchKey := fmt.Sprintf("chapters-%03d-%s", index+1, hex.EncodeToString(digest[:8]))
		path := "/openapi/v1/subject-imports/" + started.ID + "/chapters"
		if err := stagedJSONRequest(ctx, baseURL, path, token, map[string]any{"batch_key": batchKey, "chapters": batch}, dependencyClient, nil); err != nil {
			return importResult{}, err
		}
	}

	var committed struct {
		Subject struct {
			ID string `json:"id"`
		} `json:"subject"`
	}
	path := "/openapi/v1/subject-imports/" + started.ID + "/commit"
	if err := stagedJSONRequest(ctx, baseURL, path, token, map[string]any{}, dependencyClient, &committed); err != nil {
		return importResult{}, err
	}
	if !validSubjectID(committed.Subject.ID) {
		return importResult{}, fmt.Errorf("staged commit response does not contain a valid subject")
	}
	return importResult{SubjectID: committed.Subject.ID}, nil
}

func flattenImportChapters(chapters []importChapterRequest) []stagedChapter {
	result := make([]stagedChapter, 0, countImportedChapters(chapters))
	position := 0
	var appendTree func([]importChapterRequest, *string)
	appendTree = func(values []importChapterRequest, parent *string) {
		for _, value := range values {
			result = append(result, stagedChapter{
				ExternalID: value.ExternalID, ParentExternalID: parent, Position: position,
				Title: value.Title, Content: value.Content,
			})
			position++
			parentExternalID := value.ExternalID
			appendTree(value.Children, &parentExternalID)
		}
	}
	appendTree(chapters, nil)
	return result
}

func partitionStagedChapters(chapters []stagedChapter) ([][]stagedChapter, error) {
	result := make([][]stagedChapter, 0)
	current := make([]stagedChapter, 0)
	for _, chapter := range chapters {
		candidate := append(append([]stagedChapter(nil), current...), chapter)
		encoded, err := json.Marshal(map[string]any{"batch_key": "chapters-999-placeholder", "chapters": candidate})
		if err != nil {
			return nil, fmt.Errorf("encode chapter batch: %w", err)
		}
		if len(encoded) >= stagedChapterBatchBytes {
			if len(current) == 0 {
				return nil, fmt.Errorf("chapter %q exceeds the batch request limit", chapter.ExternalID)
			}
			result = append(result, current)
			current = []stagedChapter{chapter}
			continue
		}
		current = candidate
	}
	if len(current) != 0 {
		result = append(result, current)
	}
	return result, nil
}

func bundleDigest(bundle loadedBundle) (string, error) {
	type assetDigest struct {
		Key    string `json:"key"`
		SHA256 string `json:"sha256"`
	}
	assets := make([]assetDigest, 0, len(bundle.Assets))
	for _, asset := range bundle.Assets {
		digest := sha256.Sum256(asset.Content)
		assets = append(assets, assetDigest{Key: asset.Key, SHA256: hex.EncodeToString(digest[:])})
	}
	encoded, err := json.Marshal(struct {
		Request importSubjectRequest `json:"request"`
		Assets  []assetDigest        `json:"assets"`
	}{Request: bundle.Request, Assets: assets})
	if err != nil {
		return "", fmt.Errorf("encode bundle digest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func stagedJSONRequest(ctx context.Context, baseURL, path, token string, payload any, dependencyClient *http.Client, output any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode staged request: %w", err)
	}
	if len(encoded) > maxRequestBytes {
		return fmt.Errorf("staged request exceeds %d bytes", maxRequestBytes)
	}
	return stagedRequest(ctx, baseURL, path, token, "application/json", bytes.NewReader(encoded), dependencyClient, output)
}

func partitionStagedAssets(assets []localAsset) [][]localAsset {
	result := make([][]localAsset, 0, (len(assets)+stagedAssetBatchSize-1)/stagedAssetBatchSize)
	current := make([]localAsset, 0, stagedAssetBatchSize)
	currentBytes := 0
	for _, asset := range assets {
		if len(current) != 0 && (len(current) == stagedAssetBatchSize || currentBytes+len(asset.Content) > stagedAssetBatchBytes) {
			result = append(result, current)
			current = make([]localAsset, 0, stagedAssetBatchSize)
			currentBytes = 0
		}
		current = append(current, asset)
		currentBytes += len(asset.Content)
	}
	if len(current) != 0 {
		result = append(result, current)
	}
	return result
}

func stagedAssetRequest(ctx context.Context, baseURL, path, token string, assets []localAsset, dependencyClient *http.Client, output any) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, asset := range assets {
		if err := writer.WriteField("key", asset.Key); err != nil {
			return fmt.Errorf("encode asset key: %w", err)
		}
		part, err := writer.CreateFormFile("file", asset.Filename)
		if err != nil {
			return fmt.Errorf("encode asset file: %w", err)
		}
		if _, err := part.Write(asset.Content); err != nil {
			return fmt.Errorf("encode asset content: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish asset request: %w", err)
	}
	return stagedRequest(ctx, baseURL, path, token, writer.FormDataContentType(), &body, dependencyClient, output)
}

func stagedRequest(ctx context.Context, baseURL, path, token, contentType string, body io.Reader, dependencyClient *http.Client, output any) error {
	endpoint, err := apiEndpoint(baseURL, path)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return fmt.Errorf("create staged request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", contentType)
	client := http.Client{Timeout: 60 * time.Second}
	if dependencyClient != nil {
		client = *dependencyClient
		if client.Timeout == 0 {
			client.Timeout = 60 * time.Second
		}
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("send staged request: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody+1))
	if err != nil {
		return fmt.Errorf("read staged response: %w", err)
	}
	if len(responseBody) > maxResponseBody {
		return fmt.Errorf("staged response exceeds %d bytes", maxResponseBody)
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return fmt.Errorf("decode staged response (HTTP %d): %w", response.StatusCode, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.Error != nil {
		code := "request_failed"
		message := http.StatusText(response.StatusCode)
		if envelope.Error != nil {
			code = normalizeAPIErrorCode(sanitizeDiagnostic(envelope.Error.Code, token))
			message = sanitizeDiagnostic(envelope.Error.Message, token)
		}
		return fmt.Errorf("API error %s (HTTP %d): %s", code, response.StatusCode, message)
	}
	if output != nil {
		if len(envelope.Data) == 0 || json.Unmarshal(envelope.Data, output) != nil {
			return fmt.Errorf("staged response does not contain valid data")
		}
	}
	return nil
}
