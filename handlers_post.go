package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

func canPost(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	name := mux.Vars(r)["profile"]

	var p DBProfile
	if err := db.First(&p, "name = ?", name).Error; err != nil {
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
		db.Save(&p)
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
		var g DBGroupPost
		if err := db.First(&g, "group_id = ?", groupId).Error; err == nil {
			grpCooldown := config.GroupCooldownMin
			if grpCooldown <= 0 {
				grpCooldown = rand.Intn(31) + 30 // random 30–60 min
			}
			if now.Sub(g.LastPost).Minutes() < float64(grpCooldown) {
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

	// --- 3. RANDOM SKIP (shuffle fairness) ---
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
	profileLocks[name] = now.Add(30 * time.Minute)
	if groupId != "" {
		groupLocks[groupId] = now.Add(30 * time.Minute)
	}

	db.Model(&p).Update("last_ping_at", now) // implicitly they are alive if they want to post

	if !isPriority {
		fmt.Printf("✓ Profile %s grabbed post slot\n", name)
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

	var p DBProfile
	if err := db.First(&p, "name = ?", name).Error; err != nil {
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

	db.Save(&p)
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
		
		var gs DBGlobalState
		db.FirstOrCreate(&gs, DBGlobalState{ID: 1})
		db.Model(&gs).Update("next_global_post_time", now.Add(time.Duration(delay) * time.Minute))
		fmt.Printf("✓ Global cooldown set: next post after %s (+%d min)\n", gs.NextGlobalPostTime.Format("15:04:05"), delay)
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
		
		var gp DBGroupPost
		db.Where(DBGroupPost{GroupID: groupId}).Assign(DBGroupPost{LastPost: now}).FirstOrCreate(&gp)
		
		fmt.Printf("postComplete: profile=%s groupId=%s\n", name, groupId)
	} else {
		fmt.Printf("postComplete: profile=%s (no group)\n", name)
	}

	// Save Interaction for Dashboard
	targetId := groupId
	if targetId == "" {
		targetId = rawUrl
	}
	db.Create(&DBInteraction{
		Profile:   name,
		VideoID:   targetId,
		Type:      "post",
		CreatedAt: now,
	})

	w.Write([]byte("true"))
}

func canView(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	name := mux.Vars(r)["profile"]

	var p DBProfile
	if err := db.First(&p, "name = ?", name).Error; err != nil {
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
				w.Write([]byte("false"))
				return
			}
		}
	}

	hasPriorityPostedToday := false
	for _, pp := range config.PriorityProfiles {
		var pData DBProfile
		if err := db.First(&pData, "name = ?", pp).Error; err == nil {
			if now.Year() == pData.LastUpload.Year() && now.YearDay() == pData.LastUpload.YearDay() && pData.DailyCount > 0 {
				hasPriorityPostedToday = true
				break
			}
		}
	}

	if len(config.PriorityProfiles) > 0 && !hasPriorityPostedToday {
		w.Write([]byte("false"))
		return
	}

	if now.Year() != p.LastView.Year() || now.YearDay() != p.LastView.YearDay() {
		p.DailyViewCount = 0
		db.Save(&p)
	}

	limit := p.DailyViewLimit
	if limit == 0 {
		limit = 3
	}

	if p.DailyViewCount >= limit {
		fmt.Printf("Daily view limit reached for %s: %d/%d\n", name, p.DailyViewCount, limit)
		w.Write([]byte("false"))
		return
	}

	viewCooldown := p.ViewCooldownHours
	if viewCooldown == 0 {
		viewCooldown = 3
	}

	if !p.LastView.IsZero() && time.Since(p.LastView).Hours() < float64(viewCooldown) {
		fmt.Printf("View cooldown active for %s. Last view: %s\n", name, p.LastView.Format("15:04:05"))
		w.Write([]byte("false"))
		return
	}

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

	fmt.Printf("✓ Profile %s can view (%d/%d used)\n", name, p.DailyViewCount, limit)
	w.Write([]byte("true"))
}

func viewComplete(w http.ResponseWriter, r *http.Request) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	name := mux.Vars(r)["profile"]

	var p DBProfile
	if err := db.First(&p, "name = ?", name).Error; err != nil {
		fmt.Printf("Profile %s not found\n", name)
		w.Write([]byte("false"))
		return
	}

	now := time.Now()

	if now.Year() != p.LastView.Year() || now.YearDay() != p.LastView.YearDay() {
		p.DailyViewCount = 0
	}

	p.LastView = now
	p.DailyViewCount++
	db.Save(&p)

	fmt.Printf("✓ viewComplete: profile=%s dailyViewCount=%d\n", name, p.DailyViewCount)
	w.Write([]byte("true"))
}
