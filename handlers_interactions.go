package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// Helper: Count interactions today for a profile
func countDailyInteractions(profile, itype string) int64 {
	var count int64
	todayStart := time.Now().Truncate(24 * time.Hour)
	db.Model(&DBInteraction{}).Where("profile = ? AND type = ? AND created_at >= ?", profile, itype, todayStart).Count(&count)
	return count
}

func getComment(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	rawUrl := r.URL.Query().Get("url")
	profileName := mux.Vars(r)["profile"]

	if rawUrl == "" || profileName == "" {
		w.Write([]byte("false"))
		return
	}

	videoId := extractVideoId(rawUrl)
	fmt.Printf("getComment: videoId=%s profile=%s\n", videoId, profileName)

	// Check daily limit
	commentLimit := config.DailyCommentLimit
	if commentLimit <= 0 {
		commentLimit = 4
	}
	if countDailyInteractions(profileName, "comment") >= int64(commentLimit) {
		fmt.Printf("[getComment] Rejected: profile '%s' reached daily limit (%d)\n", profileName, commentLimit)
		w.Write([]byte("false"))
		return
	}

	// Check if already commented
	var count int64
	db.Model(&DBInteraction{}).Where("profile = ? AND video_id = ? AND type = ?", profileName, videoId, "comment").Count(&count)
	if count > 0 {
		fmt.Printf("[getComment] Rejected: profile '%s' already commented on video '%s'\n", profileName, videoId)
		w.Write([]byte("false"))
		return
	}

	prob := config.CommentProbability
	if prob <= 0 {
		prob = 0.3
	}
	roll := rand.Float64()
	if roll > prob {
		fmt.Printf("[getComment] Rejected: probability check failed (roll %.2f > prob %.2f)\n", roll, prob)
		w.Write([]byte("false"))
		return
	}

	selectedComment := ""

	// Try Gemini AI if HTTP POST
	if r.Method == "POST" && config.GeminiAPIKey != "" {
		var reqBody struct {
			StoryMessage string `json:"story_message"`
			GroupName    string `json:"group_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil && reqBody.StoryMessage != "" {
			fmt.Printf("Profile %s requesting AI comment via Gemini...\n", profileName)
			aiText, err := generateGeminiComment(config.GeminiAPIKey, config.GeminiModel, reqBody.StoryMessage, reqBody.GroupName)
			if err == nil && aiText != "" {
				selectedComment = aiText
				fmt.Printf("AI Comment SUCCESS: %s\n", selectedComment)
			} else {
				fmt.Printf("AI Comment failed, falling back to CSV. Error: %v\n", err)
			}
		} else {
			fmt.Printf("Decode JSON getComment failed or story_message empty: %v\n", err)
		}
	}

	// Fallback to comments.csv
	if selectedComment == "" {
		if len(comments) == 0 {
			fmt.Println("No comments loaded in CSV fallback")
			w.Write([]byte("false"))
			return
		}
		selectedComment = comments[rand.Intn(len(comments))]
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(selectedComment))
}

func commentDone(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	rawUrl := r.URL.Query().Get("url")
	profile := mux.Vars(r)["profile"]

	if rawUrl == "" || profile == "" {
		w.Write([]byte("false"))
		return
	}

	videoId := extractVideoId(rawUrl)

	// Check duplicates
	var count int64
	db.Model(&DBInteraction{}).Where("profile = ? AND video_id = ? AND type = ?", profile, videoId, "comment").Count(&count)
	if count > 0 {
		w.Write([]byte("already"))
		return
	}

	db.Create(&DBInteraction{Profile: profile, VideoID: videoId, Type: "comment", CreatedAt: time.Now()})
	fmt.Printf("commentDone: videoId=%s profile=%s\n", videoId, profile)
	w.Write([]byte("true"))
}

func canLike(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	rawUrl := r.URL.Query().Get("url")
	profileName := mux.Vars(r)["profile"]

	if rawUrl == "" || profileName == "" {
		w.Write([]byte("false"))
		return
	}

	videoId := extractVideoId(rawUrl)

	var count int64
	db.Model(&DBInteraction{}).Where("profile = ? AND video_id = ? AND type = ?", profileName, videoId, "like").Count(&count)
	if count > 0 {
		w.Write([]byte("false"))
		return
	}

	likeLimit := config.DailyLikeLimit
	if likeLimit <= 0 {
		likeLimit = 4
	}
	if countDailyInteractions(profileName, "like") >= int64(likeLimit) {
		w.Write([]byte("false"))
		return
	}

	prob := config.LikeProbability
	if prob <= 0 {
		prob = 0.4
	}
	roll := rand.Float64()
	if roll > prob {
		w.Write([]byte("false"))
		return
	}

	w.Write([]byte("true"))
}

func likeDone(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	rawUrl := r.URL.Query().Get("url")
	profile := mux.Vars(r)["profile"]

	if rawUrl == "" || profile == "" {
		w.Write([]byte("false"))
		return
	}

	videoId := extractVideoId(rawUrl)
	var count int64
	db.Model(&DBInteraction{}).Where("profile = ? AND video_id = ? AND type = ?", profile, videoId, "like").Count(&count)
	if count > 0 {
		w.Write([]byte("already"))
		return
	}

	db.Create(&DBInteraction{Profile: profile, VideoID: videoId, Type: "like", CreatedAt: time.Now()})
	w.Write([]byte("true"))
}

func videoView(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	rawUrl := r.URL.Query().Get("url")
	profile := mux.Vars(r)["profile"]

	if rawUrl == "" || profile == "" {
		w.Write([]byte("false"))
		return
	}

	videoId := extractVideoId(rawUrl)
	var count int64
	db.Model(&DBInteraction{}).Where("profile = ? AND video_id = ? AND type = ?", profile, videoId, "view").Count(&count)
	if count > 0 {
		w.Write([]byte("false"))
		return
	}

	db.Create(&DBInteraction{Profile: profile, VideoID: videoId, Type: "view", CreatedAt: time.Now()})
	w.Write([]byte("true"))
}

func canShare(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	rawUrl := r.URL.Query().Get("url")
	profile := mux.Vars(r)["profile"]

	if rawUrl == "" || profile == "" {
		w.Write([]byte("false"))
		return
	}

	videoId := extractVideoId(rawUrl)

	if len(comments) == 0 {
		w.Write([]byte("false"))
		return
	}

	dailyLimit := config.DailyShareLimit
	if dailyLimit <= 0 {
		dailyLimit = 3
	}
	if countDailyInteractions(profile, "share") >= int64(dailyLimit) {
		w.Write([]byte("false"))
		return
	}

	var shares []DBInteraction
	db.Where("video_id = ? AND type = ?", videoId, "share").Find(&shares)
	if len(shares) >= 3 {
		w.Write([]byte("false"))
		return
	}

	for _, s := range shares {
		if s.Profile == profile {
			w.Write([]byte("false"))
			return
		}
	}

	prob := config.ShareProbability
	if prob <= 0 {
		prob = 0.5
	}
	roll := rand.Float64()
	if roll > prob {
		w.Write([]byte("false"))
		return
	}

	// Pick unused comment
	var used []string
	for _, s := range shares {
		if s.Details != "" {
			used = append(used, s.Details)
		}
	}
	usedSet := map[string]bool{}
	for _, u := range used {
		usedSet[u] = true
	}

	var available []string
	for _, c := range comments {
		if !usedSet[c] {
			available = append(available, c)
		}
	}

	if len(available) == 0 {
		w.Write([]byte("false"))
		return
	}

	selectedComment := available[rand.Intn(len(available))]
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(selectedComment))
}

func shareComplete(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	rawUrl := r.URL.Query().Get("url")
	profile := mux.Vars(r)["profile"]

	if rawUrl == "" || profile == "" {
		w.Write([]byte("false"))
		return
	}

	videoId := extractVideoId(rawUrl)
	sharedComment := r.URL.Query().Get("comment")

	var count int64
	db.Model(&DBInteraction{}).Where("profile = ? AND video_id = ? AND type = ?", profile, videoId, "share").Count(&count)
	if count > 0 {
		w.Write([]byte("already"))
		return
	}

	db.Create(&DBInteraction{Profile: profile, VideoID: videoId, Type: "share", Details: sharedComment, CreatedAt: time.Now()})
	w.Write([]byte("true"))
}
