package main

import (
	"log"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

// --- Models ---

type DBProfile struct {
	Name              string `gorm:"primaryKey;column:name"`
	Status            string `gorm:"default:'Active';column:status"` // 'Active' or 'Dead'
	LastPingAt        *time.Time `gorm:"column:last_ping_at"`
	LastUpload        time.Time  `gorm:"column:last_upload"`
	CooldownHours     int        `gorm:"column:cooldown_hours"`
	DailyCount        int        `gorm:"column:daily_count"`
	DailyLimit        int        `gorm:"column:daily_limit"`
	NextPostTime      time.Time  `gorm:"column:next_post_time"`
	LastView          time.Time  `gorm:"column:last_view"`
	DailyViewCount    int        `gorm:"column:daily_view_count"`
	DailyViewLimit    int        `gorm:"column:daily_view_limit"`
	ViewCooldownHours int        `gorm:"column:view_cooldown_hours"`
}

type DBInteraction struct {
	ID        uint      `gorm:"primaryKey"`
	Profile   string    `gorm:"index"`
	VideoID   string    `gorm:"index"`
	Type      string    `gorm:"index"` // "comment", "like", "view", "share" // Removed "post" since post logic uses DBProfile LastUpload
	Details   string    // Optional details (like comment string)
	CreatedAt time.Time `gorm:"index"`
}

type DBGroupPost struct {
	GroupID  string    `gorm:"primaryKey"`
	LastPost time.Time
}

type DBGlobalState struct {
	ID                 uint `gorm:"primaryKey"`
	NextGlobalPostTime time.Time
}

func initDB() {
	var err error

	// Use silent logger by default to avoid spamming console
	db, err = gorm.Open(sqlite.Open("gpm_data.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		log.Fatal("failed to connect database:", err)
	}

	// Auto Migrate the schema
	err = db.AutoMigrate(&DBProfile{}, &DBInteraction{}, &DBGroupPost{}, &DBGlobalState{})
	if err != nil {
		log.Println("DB AutoMigrate error:", err)
	}

	// Initialize single global state row if not exists
	var gs DBGlobalState
	if db.First(&gs).Error == gorm.ErrRecordNotFound {
		db.Create(&DBGlobalState{ID: 1, NextGlobalPostTime: time.Now()})
	}
}

// Sweeper function to mark profiles as Dead if ping is older than 48 hours
func startDeadProfileSweeper() {
	go func() {
		for {
			cutoff := time.Now().Add(-48 * time.Hour)
			db.Model(&DBProfile{}).
				Where("last_ping_at < ? AND status = ?", cutoff, "Active").
				Update("status", "Dead")
			time.Sleep(1 * time.Hour) // Quét mỗi 1 tiếng
		}
	}()
}
