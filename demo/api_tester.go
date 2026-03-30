package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

var (
	serverURL   = "http://127.0.0.1:8088"
	numProfiles = 5 // X Profiles
	videoURLs   = []string{
		"https://fb.com/watch/123001",
		"https://fb.com/watch/123002",
		"https://fb.com/watch/123003",
	}
	groupURLs = []string{
		"https://fb.com/groups/456001",
		"https://fb.com/groups/456002",
	}
)

func main() {
	fmt.Printf("Bắt đầu giả lập chạy Auto cho %d Profiles...\n", numProfiles)
	rand.Seed(time.Now().UnixNano())

	var wg sync.WaitGroup

	for i := 1; i <= numProfiles; i++ {
		simulateProfile("12_thai")
	}

	wg.Wait()
	fmt.Println("🚀 Hoàn tất giả lập toàn bộ API. Hãy kiểm tra Dashboard!")
}

func simulateProfile(profile string) {
	fmt.Printf("[%s] === BẮT ĐẦU PHIÊN CHẠY ===\n", profile)

	// 1. Heartbeat
	callAPI("GET", fmt.Sprintf("/are-you-live/%s", profile), nil)
	time.Sleep(1 * time.Second)

	// 2. Lấy Session Task
	callAPI("GET", fmt.Sprintf("/get-session-tasks/%s", profile), nil)
	time.Sleep(1 * time.Second)

	// 3. Giả lập Lướt Feed & View (Ngẫu nhiên Video)
	vid := videoURLs[rand.Intn(len(videoURLs))]
	if res := callAPI("GET", fmt.Sprintf("/can-view/%s?url=%s", profile, vid), nil); res == "true" {
		callAPI("GET", fmt.Sprintf("/view-complete/%s?url=%s", profile, vid), nil)
	}

	callAPI("GET", fmt.Sprintf("/video-view/%s?url=%s", profile, vid), nil)
	time.Sleep(1 * time.Second)

	// 4. Giả lập Like
	if res := callAPI("GET", fmt.Sprintf("/can-like/%s?url=%s", profile, vid), nil); res == "true" {
		callAPI("GET", fmt.Sprintf("/like-done/%s?url=%s", profile, vid), nil)
	}

	// 5. Giả lập Share
	if res := callAPI("GET", fmt.Sprintf("/can-share/%s?url=%s", profile, vid), nil); res != "false" && len(res) > 5 {
		// Nhận được text comment share -> Call share complete
		callAPI("GET", fmt.Sprintf("/share-complete/%s?url=%s&comment=demo_share", profile, vid), nil)
	}
	time.Sleep(1 * time.Second)

	// 6. Giả lập COMMENT (CÓ GỌI AI NHƯ BẠN MUỐN)
	reqBody := map[string]string{
		"story_message": "Có bộ phim nào dạo này hay không anh em?",
		"group_name":    "Hội Mê Phim",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	commentRes := callAPI("POST", fmt.Sprintf("/get-comment/%s?url=%s", profile, vid), bodyBytes)

	if commentRes != "false" && commentRes != "" {
		// Lấy được comment -> Submit comment-done
		callAPI("GET", fmt.Sprintf("/comment-done/%s?url=%s", profile, vid), nil)
	}

	// 7. Giả lập Đăng Bài (Post)
	grp := groupURLs[rand.Intn(len(groupURLs))]
	if res := callAPI("GET", fmt.Sprintf("/can-post/%s?url=%s", profile, grp), nil); res == "true" {
		callAPI("GET", fmt.Sprintf("/post-complete/%s?url=%s", profile, grp), nil)
	}

	fmt.Printf("[%s] === KẾT THÚC PHIÊN CHẠY ===\n", profile)
}

func callAPI(method, endpoint string, body []byte) string {
	url := serverURL + endpoint
	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		fmt.Printf("Lỗi khởi tạo Request %s: %v\n", url, err)
		return "false"
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Lỗi gọi API %s: %v\n", url, err)
		return "false"
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "false"
	}

	resBody, _ := io.ReadAll(resp.Body)
	return string(resBody)
}
