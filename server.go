package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
)

// Legacy JSON struct to parse old configuration limits
type LegacyProfile struct {
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

func syncLegacyProfilesToDB() {
	var fileProfiles map[string]LegacyProfile
	file, err := os.ReadFile("profiles.json")
	if err == nil {
		json.Unmarshal(file, &fileProfiles)
		for name, pData := range fileProfiles {
			var count int64
			db.Model(&DBProfile{}).Where("name = ?", name).Count(&count)
			if count == 0 {
				now := time.Now()
				dbP := DBProfile{
					Name:              name,
					Status:            "Active",
					LastUpload:        pData.LastUpload,
					CooldownHours:     pData.CooldownHours,
					DailyCount:        pData.DailyCount,
					DailyLimit:        pData.DailyLimit,
					NextPostTime:      pData.NextPostTime,
					LastView:          pData.LastView,
					DailyViewCount:    pData.DailyViewCount,
					DailyViewLimit:    pData.DailyViewLimit,
					ViewCooldownHours: pData.ViewCooldownHours,
					LastPingAt:        &now,
				}
				db.Create(&dbP)
			}
		}
	}
}

func requireProfileMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		profile := vars["profile"]
		if profile == "" {
			profile = vars["profileName"]
		}

		if profile != "" {
			var count int64
			db.Model(&DBProfile{}).Where("name = ?", profile).Count(&count)
			if count == 0 {
				fmt.Printf("[%s] Profile not found in database. Rejected request to %s\n", profile, r.URL.Path)
				w.Write([]byte("false"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Initialize SQLite Database
	initDB()

	// Parse JSON config
	configFile, err := os.ReadFile("config.json")
	if err == nil {
		json.Unmarshal(configFile, &config)
		goldenHours = config.GoldenHours
	} else {
		goldenHours = [][]int{{7, 11}, {11, 14}, {19, 22}}
		fmt.Println("Warning: config.json not found, using defaults")
	}

	loadComments()

	// Sync profiles.json defaults to DB if not imported yet
	syncLegacyProfilesToDB()

	var count int64
	db.Model(&DBProfile{}).Count(&count)
	fmt.Println("Profiles loaded in DB:", count)
	fmt.Println("Golden Hours:", goldenHours)

	// Start 48h Sweeper
	// Start 48h Sweeper
	startDeadProfileSweeper()

	r := mux.NewRouter()

	// Dashboard API Routes
	r.HandleFunc("/api/stats/overview", apiDashboardStats).Methods("GET")
	r.HandleFunc("/api/stats/profiles", apiDashboardProfiles).Methods("GET")
	r.HandleFunc("/api/stats/profile/{profileName}/interactions", apiDashboardProfileInteractions).Methods("GET")
	r.PathPrefix("/dashboard/").Handler(http.StripPrefix("/dashboard/", http.FileServer(http.Dir("./public"))))

	// Core Automation Routes (Wrapped with Profile Checker)
	api := r.PathPrefix("/").Subrouter()
	api.Use(requireProfileMiddleware)

	api.HandleFunc("/get-session-tasks/{profile}", getSessionTasks)
	api.HandleFunc("/are-you-live/{profileName}", areYouLive)
	api.HandleFunc("/can-post/{profile}", canPost)
	api.HandleFunc("/post-complete/{profile}", postComplete)
	api.HandleFunc("/can-view/{profile}", canView)
	api.HandleFunc("/view-complete/{profile}", viewComplete)
	api.HandleFunc("/get-comment/{profile}", getComment).Methods("GET", "POST")
	api.HandleFunc("/comment-done/{profile}", commentDone)
	api.HandleFunc("/can-like/{profile}", canLike)
	api.HandleFunc("/like-done/{profile}", likeDone)
	api.HandleFunc("/video-view/{profile}", videoView)
	api.HandleFunc("/can-share/{profile}", canShare)
	api.HandleFunc("/share-complete/{profile}", shareComplete)


	fmt.Println("Server running at :8088. Dashboard: http://127.0.0.1:8088/dashboard/")
	http.ListenAndServe(":8088", r)
}
