package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/william-yangbo/kongtask/pkg/cron"
)

// WatchedCronItems provides cron items with a release function for cleanup (sync from getCronItems.ts)
type WatchedCronItems struct {
	Items   []cron.ParsedCronItem
	Release func()
}

// getCronItems loads cron items from a crontab file (simplified version of graphile-worker getCronItems.ts)
// TODO: Implement full watch mode functionality
func getCronItems(crontabFile string, watch bool) (*WatchedCronItems, error) {
	if crontabFile == "" {
		crontabFile = "./crontab"
	}

	// Check if file exists and provide intelligent logging
	if _, err := os.Stat(crontabFile); os.IsNotExist(err) {
		// Log that cron is disabled due to missing file
		log.Printf("Failed to read crontab file '%s'; cron is disabled", crontabFile)

		// Return empty cron items if file doesn't exist
		return &WatchedCronItems{
			Items:   []cron.ParsedCronItem{},
			Release: func() {}, // No-op release function
		}, nil
	}

	// Log that cron file was found
	log.Printf("Found crontab file '%s'; cron is enabled", crontabFile)

	// For now, implement basic file loading without watch mode
	// TODO: Implement proper file watching functionality
	if watch {
		return nil, fmt.Errorf("cron watch mode not yet implemented")
	}

	// Read crontab file
	cleanPath := filepath.Clean(crontabFile)
	if cleanPath != crontabFile {
		return nil, fmt.Errorf("invalid crontab file path: %s", crontabFile)
	}

	content, err := os.ReadFile(cleanPath) //#nosec G304 -- Path validated above
	if err != nil {
		// Distinguish between different types of errors
		log.Printf("Failed to read crontab file '%s': %v; cron is disabled", crontabFile, err)
		return nil, fmt.Errorf("failed to read crontab file %s: %w", crontabFile, err)
	}

	// Parse crontab content
	parser := &cron.DefaultParser{}
	cronItems, err := parser.ParseCrontab(string(content))
	if err != nil {
		log.Printf("Failed to parse crontab file '%s': %v; cron is disabled", crontabFile, err)
		return nil, fmt.Errorf("failed to parse crontab file %s: %w", crontabFile, err)
	}

	// Log successful parsing
	log.Printf("Successfully parsed crontab file '%s': found %d cron item(s)", crontabFile, len(cronItems))

	return &WatchedCronItems{
		Items:   cronItems,
		Release: func() {}, // No-op release function for now
	}, nil
}
