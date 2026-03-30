package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Copy of Models just for seeding without importing the main project
type DBProfile struct {
	Name       string `gorm:"primaryKey"`
	Status     string
	LastPingAt *time.Time
	LastUpload time.Time
}

type DBInteraction struct {
	ID        uint      `gorm:"primaryKey"`
	Profile   string    `gorm:"index"`
	VideoID   string    `gorm:"index"`
	Type      string    `gorm:"index"`
	Details   string
	CreatedAt time.Time `gorm:"index"`
}

var sampleProfiles = []string{"12_thai", "15_ngoc", "bot_01", "clone_xyz", "backup_99"}
var actionTypes = []string{"like", "comment", "share", "view", "post"}
var sampleVideos = []string{"123456789012345", "987654321098765", "https://fb.com/reel/11223344", "group_id_9999"}
var sampleComments = []string{"Hay quá bạn ơi", "Tuyệt vời", "Xin chào!", "Inb mình nhé", "Thanks admin!!"}

func main() {
	fmt.Println("GPM Demo Data Generator")
	fmt.Println("-----------------------")

	// Connect to the DB from the parent folder mapping
	db, err := gorm.Open(sqlite.Open("../gpm_data.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		fmt.Println("Failed to connect database:", err)
		return
	}

	// Make sure schemas exist
	db.AutoMigrate(&DBProfile{}, &DBInteraction{})

	rand.Seed(time.Now().UnixNano())

	// 1. Create fake profiles if they don't exist
	now := time.Now()
	for _, p := range sampleProfiles {
		lastPing := now.Add(-time.Duration(rand.Intn(100)) * time.Hour) // Random ping time
		status := "Active"
		if time.Since(lastPing).Hours() > 48 {
			status = "Dead"
		}
		
		db.FirstOrCreate(&DBProfile{Name: p}, DBProfile{
			Name:       p,
			Status:     status,
			LastPingAt: &lastPing,
			LastUpload: now.Add(-time.Duration(rand.Intn(24)) * time.Hour),
		})
	}
	fmt.Println("✅ Inserted/Verified fake profiles")

	// 2. Generate fake interactions
	fmt.Println("Generating fake interactions...")
	for i := 0; i < 150; i++ {
		prof := sampleProfiles[rand.Intn(len(sampleProfiles))]
		acType := actionTypes[rand.Intn(len(actionTypes))]
		vid := sampleVideos[rand.Intn(len(sampleVideos))]
		
		// Random time within the last 24 hours
		randomDuration := time.Duration(rand.Intn(24*60)) * time.Minute
		createdAt := time.Now().Add(-randomDuration)

		details := ""
		if acType == "comment" || acType == "share" {
			details = sampleComments[rand.Intn(len(sampleComments))]
		}

		db.Create(&DBInteraction{
			Profile:   prof,
			VideoID:   vid,
			Type:      acType,
			Details:   details,
			CreatedAt: createdAt,
		})
	}

	fmt.Println("✅ Success! 150 fake interactions generated.")
	fmt.Println("👉 Now open your Dashboard to test the Click-to-Profile popup.")
}
