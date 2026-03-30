package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

type Profile struct {
	LastUpload        time.Time `json:"last_upload"`
	CooldownHours     int       `json:"cooldown_hours"`
	DailyCount        int       `json:"daily_count"`
	DailyLimit        int       `json:"daily_limit"`
	NextPostTime      time.Time `json:"next_post_time"`
	LastView          time.Time `json:"last_view"`
	DailyViewCount    int       `json:"daily_view_count"`
	DailyViewLimit    int       `json:"daily_view_limit"`
	ViewCooldownHours int       `json:"view_cooldown_hours"`
}

type Config struct {
	GoldenHours        [][]int `json:"golden_hours"`
	GlobalCooldownMin  int     `json:"global_cooldown_min"`  // min minutes between ANY 2 posts
	GlobalCooldownMax  int     `json:"global_cooldown_max"`  // max minutes between ANY 2 posts
	ProfileCooldownMin int     `json:"profile_cooldown_min"` // min minutes between 2 posts for SAME profile
	ProfileCooldownMax int     `json:"profile_cooldown_max"` // max minutes between 2 posts for SAME profile
	ProfileMax         int     `json:"profile_max"`          // max videos per profile per day
	GroupCooldownMin   int     `json:"group_cooldown_min"`   // min minutes between posts for SAME group


	CommentProbability float64  `json:"comment_probability"`
	LikeProbability    float64  `json:"like_probability"`
	ShareProbability   float64  `json:"share_probability"`
	ViewProbability    float64  `json:"view_probability"`
	DailyShareLimit    int      `json:"daily_share_limit"`
	DailyCommentLimit  int      `json:"daily_comment_limit"`
	DailyLikeLimit     int      `json:"daily_like_limit"`
	PriorityProfiles   []string `json:"priority_profiles"`
	PostSkipChance     float64  `json:"post_skip_chance"` // 0.0–1.0: chance a passing profile is skipped for fairness (default 0.4)
}

var globalMutex sync.Mutex
var profiles map[string]Profile
var config Config
var goldenHours [][]int

var profileLocks = make(map[string]time.Time)
var groupLocks = make(map[string]time.Time)

type globalPostState struct {
	NextGlobalPostTime time.Time `json:"next_global_post_time"`
}

var currentGlobalPostState = &globalPostState{}

func loadGlobalPostState() {
	data, err := os.ReadFile("global_post_state.json")
	if err != nil {
		return
	}
	json.Unmarshal(data, currentGlobalPostState)
}

func saveGlobalPostState() {
	data, _ := json.MarshalIndent(currentGlobalPostState, "", "  ")
	os.WriteFile("global_post_state.json", data, 0644)
}


// commentHistory tracks which profiles have already commented on a videoId.
// map[videoId][]profileName
var commentHistory map[string][]string

// likeHistory tracks which profiles have already liked a videoId.
var likeHistory map[string][]string

// viewHistory tracks which profiles have already viewed a videoId.
var viewHistory map[string][]string

// groupPostHistory tracks the last post time per groupId.
var groupPostHistory map[string]time.Time

// shareHistory tracks which profiles have shared a videoId.
// map[videoId][]profileName — max 3 per videoId
var shareHistory map[string][]string

// shareUsedComments tracks which comments have been used for each videoId.
// map[videoId][]comment
var shareUsedComments map[string][]string

// profileDailyShares tracks how many shares each profile has done today.
// map[profileName]{count, date}
type dailyShareInfo struct {
	Count int
	Date  string // "2006-01-02"
}

var profileDailyShares = map[string]*dailyShareInfo{}
var profileDailyComments = map[string]*dailyShareInfo{}
var profileDailyLikes = map[string]*dailyShareInfo{}

// comments holds all lines loaded from comments.csv
var comments []string

var videoIdRegex = regexp.MustCompile(`[^/\\?&=]+$`)

// extractVideoId extracts the video/reel ID from a Facebook URL.
// e.g. "https://www.facebook.com/reel/1232463431851646" → "1232463431851646"
func extractVideoId(rawUrl string) string {
	// Strip any query string
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

func loadCommentHistory() {
	commentHistory = map[string][]string{}
	data, err := os.ReadFile("comments_history.json")
	if err != nil {
		return
	}
	json.Unmarshal(data, &commentHistory)
}

func saveCommentHistory() {
	data, _ := json.MarshalIndent(commentHistory, "", "  ")
	os.WriteFile("comments_history.json", data, 0644)
}

func loadLikeHistory() {
	likeHistory = map[string][]string{}
	data, err := os.ReadFile("likes_history.json")
	if err != nil {
		return
	}
	json.Unmarshal(data, &likeHistory)
}

func saveLikeHistory() {
	data, _ := json.MarshalIndent(likeHistory, "", "  ")
	os.WriteFile("likes_history.json", data, 0644)
}

func loadViewHistory() {
	viewHistory = map[string][]string{}
	data, err := os.ReadFile("views_history.json")
	if err != nil {
		return
	}
	json.Unmarshal(data, &viewHistory)
}

func saveViewHistory() {
	data, _ := json.MarshalIndent(viewHistory, "", "  ")
	os.WriteFile("views_history.json", data, 0644)
}

func loadGroupPostHistory() {
	groupPostHistory = map[string]time.Time{}
	data, err := os.ReadFile("group_post_history.json")
	if err != nil {
		return
	}
	json.Unmarshal(data, &groupPostHistory)
}

func saveGroupPostHistory() {
	data, _ := json.MarshalIndent(groupPostHistory, "", "  ")
	os.WriteFile("group_post_history.json", data, 0644)
}

func loadShareHistory() {
	shareHistory = map[string][]string{}
	shareUsedComments = map[string][]string{}
	data, err := os.ReadFile("shares_history.json")
	if err != nil {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	if v, ok := raw["profiles"]; ok {
		json.Unmarshal(v, &shareHistory)
	}
	if v, ok := raw["used_comments"]; ok {
		json.Unmarshal(v, &shareUsedComments)
	}
}

func saveShareHistory() {
	data, _ := json.MarshalIndent(map[string]interface{}{
		"profiles":      shareHistory,
		"used_comments": shareUsedComments,
	}, "", "  ")
	os.WriteFile("shares_history.json", data, 0644)
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

func saveProfiles() {
	data, _ := json.MarshalIndent(profiles, "", "  ")
	os.WriteFile("profiles.json", data, 0644)
}

func logHistory(profile string) {
	f, _ := os.OpenFile("history.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()
	entry := fmt.Sprintf("%s|%s\n", time.Now().Format("2006-01-02 15:04:05"), profile)
	f.WriteString(entry)
}



// getComment decides if the calling profile should comment on this video.
// Query params: url (Facebook URL), profile (the profile asking)
// - Extracts videoId from url
// - If profile already commented on this video → false
// - Rolls random probability (config.comment_probability): if not selected → false
// - If selected → returns the comment to post
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

	if len(comments) == 0 {
		fmt.Println("No comments loaded")
		w.Write([]byte("false"))
		return
	}

	// Check daily comment limit per profile
	commentLimit := config.DailyCommentLimit
	if commentLimit <= 0 {
		commentLimit = 4
	}
	today := time.Now().Format("2006-01-02")
	dc := profileDailyComments[profileName]
	if dc != nil && dc.Date == today && dc.Count >= commentLimit {
		fmt.Printf("Profile %s reached daily comment limit (%d/%d)\n", profileName, dc.Count, commentLimit)
		w.Write([]byte("false"))
		return
	}

	// Check if this profile already commented on this video
	for _, p := range commentHistory[videoId] {
		if p == profileName {
			fmt.Printf("Profile %s already commented on videoId=%s\n", profileName, videoId)
			w.Write([]byte("false"))
			return
		}
	}

	// Apply random probability — not every profile that asks will comment
	prob := config.CommentProbability
	if prob <= 0 {
		prob = 0.3 // default 30%
	}
	roll := rand.Float64()
	if roll > prob {
		fmt.Printf("Profile %s not selected to comment (roll=%.2f > prob=%.2f)\n", profileName, roll, prob)
		w.Write([]byte("false"))
		return
	}

	// Selected! Return a random comment
	selectedComment := comments[rand.Intn(len(comments))]
	fmt.Printf("Profile %s selected to comment on videoId=%s (roll=%.2f <= prob=%.2f)\n", profileName, videoId, roll, prob)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(selectedComment))
}

// commentDone marks a profile as having commented on a video.
// Query params: videoId, profile
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
	// Avoid duplicates
	for _, p := range commentHistory[videoId] {
		if p == profile {
			w.Write([]byte("already"))
			return
		}
	}

	commentHistory[videoId] = append(commentHistory[videoId], profile)
	saveCommentHistory()
	incrementDaily(profileDailyComments, profile)
	fmt.Printf("commentDone: videoId=%s profile=%s\n", videoId, profile)
	w.Write([]byte("true"))
}

// canLike decides if the calling profile should like this video.
// Query params: url (Facebook URL), profile (the profile asking)
// - If already liked → false
// - Rolls random like_probability → true/false
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
	fmt.Printf("canLike: videoId=%s profile=%s\n", videoId, profileName)

	// Check if already liked
	for _, p := range likeHistory[videoId] {
		if p == profileName {
			fmt.Printf("Profile %s already liked videoId=%s\n", profileName, videoId)
			w.Write([]byte("false"))
			return
		}
	}

	// Check daily like limit per profile
	likeLimit := config.DailyLikeLimit
	if likeLimit <= 0 {
		likeLimit = 4
	}
	today := time.Now().Format("2006-01-02")
	dl := profileDailyLikes[profileName]
	if dl != nil && dl.Date == today && dl.Count >= likeLimit {
		fmt.Printf("Profile %s reached daily like limit (%d/%d)\n", profileName, dl.Count, likeLimit)
		w.Write([]byte("false"))
		return
	}

	// Apply random probability
	prob := config.LikeProbability
	if prob <= 0 {
		prob = 0.4 // default 40%
	}
	roll := rand.Float64()
	if roll > prob {
		fmt.Printf("Profile %s not selected to like (roll=%.2f > prob=%.2f)\n", profileName, roll, prob)
		w.Write([]byte("false"))
		return
	}

	fmt.Printf("Profile %s selected to like videoId=%s (roll=%.2f <= prob=%.2f)\n", profileName, videoId, roll, prob)
	w.Write([]byte("true"))
}

// likeDone marks a profile as having liked a video.
// Query params: videoId, profile
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
	// Avoid duplicates
	for _, p := range likeHistory[videoId] {
		if p == profile {
			w.Write([]byte("already"))
			return
		}
	}

	likeHistory[videoId] = append(likeHistory[videoId], profile)
	saveLikeHistory()
	incrementDaily(profileDailyLikes, profile)
	fmt.Printf("likeDone: videoId=%s profile=%s\n", videoId, profile)
	w.Write([]byte("true"))
}

// videoView checks if a profile has already viewed this video.
// If already viewed → returns "false". If not → saves and returns "true".
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
	fmt.Printf("videoView: videoId=%s profile=%s\n", videoId, profile)

	// Check if already viewed
	for _, p := range viewHistory[videoId] {
		if p == profile {
			fmt.Printf("Profile %s already viewed videoId=%s\n", profile, videoId)
			w.Write([]byte("false"))
			return
		}
	}

	// Not viewed yet → save and return true
	viewHistory[videoId] = append(viewHistory[videoId], profile)
	saveViewHistory()
	fmt.Printf("videoView saved: videoId=%s profile=%s\n", videoId, profile)
	w.Write([]byte("true"))
}

// canShare checks if a video can be shared by this profile.
// Returns "false" if not allowed, or a unique comment string to use when sharing.
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
	fmt.Printf("canShare: videoId=%s profile=%s\n", videoId, profile)

	if len(comments) == 0 {
		fmt.Println("No comments loaded")
		w.Write([]byte("false"))
		return
	}

	// Check daily share limit per profile
	dailyLimit := config.DailyShareLimit
	if dailyLimit <= 0 {
		dailyLimit = 3
	}
	today := time.Now().Format("2006-01-02")
	ds := profileDailyShares[profile]
	if ds != nil && ds.Date == today && ds.Count >= dailyLimit {
		fmt.Printf("Profile %s reached daily share limit (%d/%d)\n", profile, ds.Count, dailyLimit)
		w.Write([]byte("false"))
		return
	}

	shares := shareHistory[videoId]

	// Max 3 shares per video
	if len(shares) >= 3 {
		fmt.Printf("Video %s already has %d shares (max 3)\n", videoId, len(shares))
		w.Write([]byte("false"))
		return
	}

	// Check if this profile already shared
	for _, p := range shares {
		if p == profile {
			fmt.Printf("Profile %s already shared videoId=%s\n", profile, videoId)
			w.Write([]byte("false"))
			return
		}
	}

	// Apply random probability
	prob := config.ShareProbability
	if prob <= 0 {
		prob = 0.5 // default 50%
	}
	roll := rand.Float64()
	if roll > prob {
		fmt.Printf("Profile %s not selected to share (roll=%.2f > prob=%.2f)\n", profile, roll, prob)
		w.Write([]byte("false"))
		return
	} else {
		fmt.Printf("Profile %s selected to share (roll=%.2f <= prob=%.2f)\n", profile, roll, prob)
	}

	// Pick a random comment not already used for this video
	usedSet := map[string]bool{}
	for _, c := range shareUsedComments[videoId] {
		usedSet[c] = true
	}

	var available []string
	for _, c := range comments {
		if !usedSet[c] {
			available = append(available, c)
		}
	}

	if len(available) == 0 {
		fmt.Printf("No unused comments left for videoId=%s\n", videoId)
		w.Write([]byte("false"))
		return
	}

	selectedComment := available[rand.Intn(len(available))]

	// NOTE: do NOT save here — shareComplete will persist the used comment
	// This prevents marking a comment as used when the share fails.
	fmt.Printf("Profile %s can share videoId=%s (%d/3), comment: %s\n", profile, videoId, len(shares), selectedComment)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(selectedComment))
}

func incrementDaily(m map[string]*dailyShareInfo, profile string) {
	today := time.Now().Format("2006-01-02")
	ds := m[profile]
	if ds == nil || ds.Date != today {
		m[profile] = &dailyShareInfo{Count: 1, Date: today}
	} else {
		ds.Count++
	}
}

// shareComplete marks a profile as having shared a video.
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

	// Avoid duplicates
	for _, p := range shareHistory[videoId] {
		if p == profile {
			w.Write([]byte("already"))
			return
		}
	}

	shareHistory[videoId] = append(shareHistory[videoId], profile)

	// Now persist the used comment (moved from canShare)
	if sharedComment != "" {
		shareUsedComments[videoId] = append(shareUsedComments[videoId], sharedComment)
	}

	saveShareHistory()
	incrementDaily(profileDailyShares, profile)
	fmt.Printf("shareComplete: videoId=%s profile=%s (%d/3)\n", videoId, profile, len(shareHistory[videoId]))
	w.Write([]byte("true"))
}

func canPost(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	name := mux.Vars(r)["profile"]

	p, ok := profiles[name]
	if !ok {
		fmt.Printf("Profile %s not found\n", name)
		w.Write([]byte("false"))
		return
	}

	now := time.Now()

	_, inGolden := currentGoldenHourStart()
	if !inGolden {
		fmt.Printf("Profile %s: Not in golden hour. Current hour: %d\n", name, time.Now().Hour())
		w.Write([]byte("false"))
		return
	}

	isPriority := false
	for _, pp := range config.PriorityProfiles {
		if pp == name {
			isPriority = true
			break
		}
	}

	// Reset DailyCount if new day
	if now.Year() != p.LastUpload.Year() || now.YearDay() != p.LastUpload.YearDay() {
		p.DailyCount = 0
	}

	limit := p.DailyLimit
	if limit <= 0 {
		limit = config.ProfileMax
		if limit <= 0 {
			limit = 3
		}
	}

	if !isPriority && p.DailyCount >= limit {
		fmt.Printf("Daily limit reached for %s: %d/%d\n", name, p.DailyCount, limit)
		w.Write([]byte("false"))
		return
	}

	// --- 1. PROFILE COOLDOWN & LOCK ---
	if !p.NextPostTime.IsZero() {
		if now.Before(p.NextPostTime) {
			fmt.Printf("Profile %s is on cooldown until: %s\n", name, p.NextPostTime.Format("15:04:05"))
			w.Write([]byte("false"))
			return
		}
	} else {
		profCooldown := config.ProfileCooldownMin
		if profCooldown <= 0 {
			profCooldown = 180 // default 3 hours between posts for a single account
		}
		if now.Sub(p.LastUpload).Minutes() < float64(profCooldown) {
			fmt.Printf("Profile %s is on cooldown. Wait mins: %d\n", name, profCooldown)
			w.Write([]byte("false"))
			return
		}
	}
	// Check temporary memory lock
	if lockTime, exists := profileLocks[name]; exists && now.Before(lockTime) {
		fmt.Printf("Profile %s holds a temporary lock until %s\n", name, lockTime.Format("15:04:05"))
		w.Write([]byte("false"))
		return
	}

	// --- 1.5 GROUP COOLDOWN ---
	groupUrl := r.URL.Query().Get("group")
	var groupId string
	if groupUrl != "" {
		groupId = extractVideoId(groupUrl)
	}

	if groupId != "" && !isPriority {
		if lastGroupPost, exists := groupPostHistory[groupId]; exists {
			grpCooldown := config.GroupCooldownMin
			if grpCooldown <= 0 {
				grpCooldown = rand.Intn(31) + 30 // random 30–60 min
			}
			if now.Sub(lastGroupPost).Minutes() < float64(grpCooldown) {
				fmt.Printf("Group %s is on cooldown. Wait mins: %d\n", groupId, grpCooldown)
				w.Write([]byte("false"))
				return
			}
		}
		// Check temporary memory lock for group
		if lockTime, exists := groupLocks[groupId]; exists && now.Before(lockTime) {
			w.Write([]byte("false"))
			return
		}
	}

	// --- 2. GLOBAL COOLDOWN ---
	if !isPriority && now.Before(currentGlobalPostState.NextGlobalPostTime) {
		// Wait for next global slot
		w.Write([]byte("false"))
		return
	}

	// --- 3. RANDOM SKIP (shuffle fairness) ---
	// Even if all conditions pass, non-priority profiles have a chance to be skipped.
	// This prevents profiles called first in a loop from always grabbing the slot.
	// Default: 40% skip chance → on average, ~2-3 profiles are polled before one wins.
	if !isPriority {
		skipChance := config.PostSkipChance
		if skipChance <= 0 {
			skipChance = 0.4
		}
		if rand.Float64() < skipChance {
			fmt.Printf("↷ Profile %s skipped this round (random fairness)\n", name)
			w.Write([]byte("false"))
			return
		}
	}

	// --- SUCCESS! ---
	// Lock the profile temporarily for 30 minutes in memory to give it time to upload and trigger postComplete.
	profileLocks[name] = now.Add(30 * time.Minute)
	if groupId != "" {
		groupLocks[groupId] = now.Add(30 * time.Minute)
	}

	if !isPriority {
		fmt.Printf("✓ Profile %s grabbed post slot (global cooldown will be set after upload completes)\n", name)
	} else {
		fmt.Printf("✓ Priority Profile %s grabbed post slot.\n", name)
	}

	w.Write([]byte("true"))
}

func postComplete(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	name := mux.Vars(r)["profile"]
	rawUrl := r.URL.Query().Get("url")

	p, ok := profiles[name]
	if !ok {
		fmt.Printf("Profile %s not found\n", name)
		w.Write([]byte("false"))
		return
	}

	now := time.Now()

	// Reset DailyCount if new day
	if now.Year() != p.LastUpload.Year() || now.YearDay() != p.LastUpload.YearDay() {
		p.DailyCount = 0
	}

	// Update profile state
	p.LastUpload = now
	p.DailyCount++

	profMin := config.ProfileCooldownMin
	if profMin <= 0 {
		profMin = 100 // 3 hours
	}
	profMax := config.ProfileCooldownMax
	if profMax <= 0 {
		profMax = 120 // 4 hours
	}
	if profMax < profMin {
		profMax = profMin
	}
	delay := rand.Intn(profMax-profMin+1) + profMin
	p.NextPostTime = now.Add(time.Duration(delay) * time.Minute)

	profiles[name] = p
	saveProfiles()
	logHistory(name)

	delete(profileLocks, name)

	// Set global cooldown now that upload actually succeeded
	isPriority := false
	for _, pp := range config.PriorityProfiles {
		if pp == name {
			isPriority = true
			break
		}
	}
	if !isPriority {
		gMin := config.GlobalCooldownMin
		if gMin <= 0 {
			gMin = 15
		}
		gMax := config.GlobalCooldownMax
		if gMax <= 0 {
			gMax = 45
		}
		if gMax < gMin {
			gMax = gMin
		}
		delay := rand.Intn(gMax-gMin+1) + gMin
		currentGlobalPostState.NextGlobalPostTime = now.Add(time.Duration(delay) * time.Minute)
		saveGlobalPostState()
		fmt.Printf("✓ Global cooldown set: next post after %s (+%d min)\n", currentGlobalPostState.NextGlobalPostTime.Format("15:04:05"), delay)
	}

	groupUrl := r.URL.Query().Get("group")
	var groupId string
	if groupUrl != "" {
		groupId = extractVideoId(groupUrl)
	} else if rawUrl != "" {
		groupId = extractVideoId(rawUrl)
	}

	if groupId != "" {
		delete(groupLocks, groupId)
		groupPostHistory[groupId] = now
		saveGroupPostHistory()
		fmt.Printf("postComplete: profile=%s groupId=%s\n", name, groupId)
	} else {
		fmt.Printf("postComplete: profile=%s (no group)\n", name)
	}

	w.Write([]byte("true"))
}

func canView(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	name := mux.Vars(r)["profile"]

	p, ok := profiles[name]
	if !ok {
		fmt.Printf("Profile %s not found\n", name)
		w.Write([]byte("false"))
		return
	}

	now := time.Now()

	// Block viewing between 01:00 – 06:00 (night hours)
	if h := now.Hour(); h >= 1 && h < 6 {
		w.Write([]byte("false"))
		return
	}

	if _, inGolden := currentGoldenHourStart(); inGolden {
		for _, pp := range config.PriorityProfiles {
			if pp == name {
				// Priority profiles should save their threads for posting.
				w.Write([]byte("false"))
				return
			}
		}
	}

	// BLOCKS `canView` FOR EVERYONE IF NO PRIORITY PROFILE HAS POSTED TODAY
	hasPriorityPostedToday := false
	for _, pp := range config.PriorityProfiles {
		if ppData, exists := profiles[pp]; exists {
			if now.Year() == ppData.LastUpload.Year() && now.YearDay() == ppData.LastUpload.YearDay() && ppData.DailyCount > 0 {
				hasPriorityPostedToday = true
				break
			}
		}
	}

	if len(config.PriorityProfiles) > 0 && !hasPriorityPostedToday {
		// fmt.Printf("canView Blocked: Priority Profiles have not posted yet today.\n")
		w.Write([]byte("false"))
		return
	}

	// Reset DailyViewCount if new day — MUST run before limit/cooldown checks
	if now.Year() != p.LastView.Year() || now.YearDay() != p.LastView.YearDay() {
		p.DailyViewCount = 0
	}

	// Default limit if 0
	limit := p.DailyViewLimit
	if limit == 0 {
		limit = 3
	}

	if p.DailyViewCount >= limit {
		fmt.Printf("Daily view limit reached for %s: %d/%d\n", name, p.DailyViewCount, limit)
		w.Write([]byte("false"))
		return
	}

	// Default cooldown if 0
	viewCooldown := p.ViewCooldownHours
	if viewCooldown == 0 {
		viewCooldown = 3
	}

	// Guard zero value — new profile has never viewed
	if !p.LastView.IsZero() && time.Since(p.LastView).Hours() < float64(viewCooldown) {
		fmt.Printf("View cooldown active for %s. Last view: %s\n", name, p.LastView.Format("15:04:05"))
		w.Write([]byte("false"))
		return
	}

	// Apply random probability for viewing
	prob := config.ViewProbability
	if prob <= 0 {
		prob = 0.8
	}
	roll := rand.Float64()
	if roll > prob {
		fmt.Printf("Profile %s not selected to view (roll=%.2f > prob=%.2f)\n", name, roll, prob)
		w.Write([]byte("false"))
		return
	}

	// State update is moved to /view-complete/{profile}
	fmt.Printf("✓ Profile %s can view (%d/%d used)\n", name, p.DailyViewCount, limit)
	w.Write([]byte("true"))
}

func viewComplete(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	name := mux.Vars(r)["profile"]

	p, ok := profiles[name]
	if !ok {
		fmt.Printf("Profile %s not found\n", name)
		w.Write([]byte("false"))
		return
	}

	now := time.Now()

	// Reset DailyViewCount if new day
	if now.Year() != p.LastView.Year() || now.YearDay() != p.LastView.YearDay() {
		p.DailyViewCount = 0
	}

	// Update state
	p.LastView = now
	p.DailyViewCount++
	profiles[name] = p

	saveProfiles()
	fmt.Printf("✓ viewComplete: profile=%s dailyViewCount=%d\n", name, p.DailyViewCount)
	w.Write([]byte("true"))
}

func main() {
	// Load Profiles
	file, _ := os.ReadFile("profiles.json")
	json.Unmarshal(file, &profiles)

	// Load Config
	configFile, err := os.ReadFile("config.json")
	if err == nil {
		json.Unmarshal(configFile, &config)
		goldenHours = config.GoldenHours
	} else {
		// Default if config missing
		goldenHours = [][]int{{7, 11}, {11, 14}, {19, 22}}
		fmt.Println("Warning: config.json not found, using defaults")
	}

	// Load comments
	loadComments()
	loadCommentHistory()
	loadLikeHistory()
	loadViewHistory()
	loadGroupPostHistory()
	loadShareHistory()
	loadGlobalPostState()

	fmt.Println("Profiles loaded:", len(profiles))
	fmt.Println("Golden Hours:", goldenHours)

	r := mux.NewRouter()

	r.HandleFunc("/can-post/{profile}", canPost)
	r.HandleFunc("/post-complete/{profile}", postComplete)

	r.HandleFunc("/can-view/{profile}", canView)
	r.HandleFunc("/view-complete/{profile}", viewComplete)

	r.HandleFunc("/get-comment/{profile}", getComment)
	r.HandleFunc("/comment-done/{profile}", commentDone)

	r.HandleFunc("/can-like/{profile}", canLike)
	r.HandleFunc("/like-done/{profile}", likeDone)

	r.HandleFunc("/video-view/{profile}", videoView)

	r.HandleFunc("/can-share/{profile}", canShare)
	r.HandleFunc("/share-complete/{profile}", shareComplete)

	http.ListenAndServe(":8080", r)
}
