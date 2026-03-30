package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type ManageFriendsTask struct {
	AcceptRequest  int `json:"accept_request"`
	RequestFriends int `json:"request_friends"`
}

type DefaultTask struct {
}

type SessionTasksResponse struct {
	CheckNotifications bool               `json:"check_notifications,omitempty"`
	ManageFriends      *ManageFriendsTask `json:"manage_friends,omitempty"`
	ViewStories        *DefaultTask       `json:"view_stories,omitempty"`
	WatchReels         *DefaultTask       `json:"watch_reels,omitempty"`
	ViewFeed           *DefaultTask       `json:"view_feed,omitempty"`
}

func getSessionTasks(w http.ResponseWriter, r *http.Request) {
	profile := mux.Vars(r)["profile"]
	if profile == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write([]byte("{}"))
		return
	}

	roll := rand.Float64()
	var template []string

	if roll < 0.40 {
		template = append(template, "check_notifications", "view_feed")
	} else if roll < 0.70 {
		template = append(template, "watch_reels", "view_stories")
	} else if roll < 0.90 {
		template = append(template, "manage_friends", "view_feed")
	} else {
		template = append(template, "check_notifications", "manage_friends", "view_stories", "watch_reels")
	}

	resp := SessionTasksResponse{}
	dt := &DefaultTask{}

	for _, t := range template {
		switch t {
		case "check_notifications":
			resp.CheckNotifications = true
		case "view_feed":
			resp.ViewFeed = dt
		case "watch_reels":
			resp.WatchReels = dt
		case "view_stories":
			resp.ViewStories = dt
		case "manage_friends":
			resp.ManageFriends = &ManageFriendsTask{
				AcceptRequest:  rand.Intn(4),
				RequestFriends: rand.Intn(6) + 1,
			}
		}
	}

	resBytes, _ := json.Marshal(resp)
	fmt.Printf("✓ Profile %s received session tasks (JSON): %s\n", profile, string(resBytes))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(resBytes)
}

func areYouLive(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	profile := mux.Vars(r)["profileName"]

	if profile == "" {
		w.Write([]byte("false"))
		return
	}

	var p DBProfile
	if err := db.First(&p, "name = ?", profile).Error; err != nil {
		w.Write([]byte("false"))
		return
	}

	now := time.Now()
	p.LastPingAt = &now
	p.Status = "Active"
	db.Save(&p)

	w.Write([]byte("true"))
}

// --- Dashboard API ---

type DashboardStat struct {
	ActiveProfiles int64 `json:"active_profiles"`
	DeadProfiles   int64 `json:"dead_profiles"`
	LikesToday     int64 `json:"likes_today"`
	CommentsToday  int64 `json:"comments_today"`
	SharesToday    int64 `json:"shares_today"`
	ViewsToday     int64 `json:"views_today"`
}

func apiDashboardStats(w http.ResponseWriter, r *http.Request) {
	var stat DashboardStat
	db.Model(&DBProfile{}).Where("status = ?", "Active").Count(&stat.ActiveProfiles)
	db.Model(&DBProfile{}).Where("status = ?", "Dead").Count(&stat.DeadProfiles)

	todayStart := time.Now().Truncate(24 * time.Hour)
	
	db.Model(&DBInteraction{}).Where("type = ? AND created_at >= ?", "like", todayStart).Count(&stat.LikesToday)
	db.Model(&DBInteraction{}).Where("type = ? AND created_at >= ?", "comment", todayStart).Count(&stat.CommentsToday)
	db.Model(&DBInteraction{}).Where("type = ? AND created_at >= ?", "share", todayStart).Count(&stat.SharesToday)
	db.Model(&DBInteraction{}).Where("type = ? AND created_at >= ?", "view", todayStart).Count(&stat.ViewsToday)

	w.Header().Set("Content-Type", "application/json")
	json.MarshalIndent(stat, "", "  ")
	b, _ := json.Marshal(stat)
	w.Write(b)
}

func apiDashboardProfiles(w http.ResponseWriter, r *http.Request) {
	var profiles []DBProfile
	db.Order("last_ping_at DESC").Find(&profiles)
	
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(profiles)
	w.Write(b)
}

type ProfileInteractionDetail struct {
	Type      string `json:"type"`
	VideoID   string `json:"video_id"`
	Details   string `json:"details"`
	CreatedAt string `json:"created_at"`
}

func apiDashboardProfileInteractions(w http.ResponseWriter, r *http.Request) {
	profile := mux.Vars(r)["profileName"]
	if profile == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var interactions []DBInteraction
	// Lấy 50 tương tác gần nhất của profile này
	db.Where("profile = ?", profile).Order("created_at DESC").Limit(50).Find(&interactions)

	var results []ProfileInteractionDetail
	for _, it := range interactions {
		results = append(results, ProfileInteractionDetail{
			Type:      it.Type,
			VideoID:   it.VideoID,
			Details:   it.Details,
			CreatedAt: it.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if len(results) == 0 {
		w.Write([]byte("[]"))
	} else {
		b, _ := json.Marshal(results)
		w.Write(b)
	}
}
