package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Config struct {
	GoldenHours        [][]int  `json:"golden_hours"`
	GlobalCooldownMin  int      `json:"global_cooldown_min"`
	GlobalCooldownMax  int      `json:"global_cooldown_max"`
	ProfileCooldownMin int      `json:"profile_cooldown_min"`
	ProfileCooldownMax int      `json:"profile_cooldown_max"`
	ProfileMax         int      `json:"profile_max"`
	GroupCooldownMin   int      `json:"group_cooldown_min"`
	CommentProbability float64  `json:"comment_probability"`
	LikeProbability    float64  `json:"like_probability"`
	ShareProbability   float64  `json:"share_probability"`
	ViewProbability    float64  `json:"view_probability"`
	DailyShareLimit    int      `json:"daily_share_limit"`
	DailyCommentLimit  int      `json:"daily_comment_limit"`
	DailyLikeLimit     int      `json:"daily_like_limit"`
	PriorityProfiles   []string `json:"priority_profiles"`
	PostSkipChance     float64  `json:"post_skip_chance"`
	GeminiAPIKey       string   `json:"gemini_api_key"`
	GeminiModel        string   `json:"gemini_model"`
}

var globalMutex sync.Mutex
var config Config
var goldenHours [][]int

var profileLocks = make(map[string]time.Time)
var groupLocks = make(map[string]time.Time)

var comments []string
var videoIdRegex = regexp.MustCompile(`[^/\\?&=]+$`)

func extractVideoId(rawUrl string) string {
	if idx := strings.Index(rawUrl, "?"); idx != -1 {
		rawUrl = rawUrl[:idx]
	}
	rawUrl = strings.TrimRight(rawUrl, "/")
	parts := strings.Split(rawUrl, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return rawUrl
}

func loadComments() {
	f, err := os.Open("comments.csv")
	if err != nil {
		fmt.Println("Warning: could not open comments.csv:", err)
		return
	}
	defer f.Close()

	comments = nil
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			comments = append(comments, line)
		}
	}
	fmt.Printf("Comments loaded: %d\n", len(comments))
}

func currentGoldenHourStart() (int, bool) {
	h := time.Now().Hour()
	for _, g := range goldenHours {
		if h >= g[0] && h < g[1] {
			return g[0], true
		}
	}
	return 0, false
}

func logHistory(profile string) {
	f, _ := os.OpenFile("history.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()
	entry := fmt.Sprintf("%s|%s\n", time.Now().Format("2006-01-02 15:04:05"), profile)
	f.WriteString(entry)
}
