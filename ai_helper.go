package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func generateGeminiComment(apiKey, model, storyMessage, groupName string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("Gemini API Key is empty")
	}

	if model == "" {
		model = "gemini-1.5-flash"
	}

	// 1. Build the prompt
	prompt := fmt.Sprintf(
		"Bạn là một người dùng mạng xã hội bình thường. " +
		"Hãy nhận diện ngôn ngữ của bài viết này và hãy trả lời bằng ĐÚNG NGÔN NGỮ ĐÓ (ví dụ nếu nội dung tiếng Thái, bạn phải comment bằng tiếng Thái). " +
		"Dưới đây là nội dung một bài viết: '%s'. ", storyMessage)

	if groupName != "" {
		prompt += fmt.Sprintf("Bài viết thuộc nhóm: '%s'. ", groupName)
	}

	prompt += "Hãy viết 1 câu bình luận cực ngắn gọn, tự nhiên, giống một cá nhân thật đang lướt mạng. " +
		"Không dùng cấu trúc khuôn mẫu, không dùng Hashtag hay emoji quá đà."

	// 2. Prepare payload
	reqData := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{
					{Text: prompt},
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(reqData)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	// 3. Execute HTTP Call
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 4. Parse Response
	var geminiResp GeminiResponse
	if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
		return "", fmt.Errorf("Failed to parse response: %s", string(respBytes))
	}

	if geminiResp.Error != nil {
		return "", fmt.Errorf("API Error: %s", geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		text := geminiResp.Candidates[0].Content.Parts[0].Text
		text = strings.TrimSpace(text)
		return text, nil
	}

	return "", fmt.Errorf("Empty response from Gemini")
}
