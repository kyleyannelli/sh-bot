package main

import (
	"os"
	"sync"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestValidateScript(t *testing.T) {
	t.Run("EmptyScriptPath", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic for empty script path")
			}
		}()
		validateScript("")
	})

	t.Run("NonExistentFile", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic for non-existent file")
			}
		}()
		validateScript("/non/existent/path")
	})

	t.Run("NonExecutableFile", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "testfile")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())
		// Ensure the file is not executable
		if err := os.Chmod(tmpfile.Name(), 0644); err != nil {
			t.Fatal(err)
		}

		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic for non-executable file")
			}
		}()
		validateScript(tmpfile.Name())
	})

	t.Run("ValidExecutableFile", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "testfile")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())
		// Make the file executable
		if err := os.Chmod(tmpfile.Name(), 0755); err != nil {
			t.Fatal(err)
		}

		// Should not panic
		validateScript(tmpfile.Name())
	})
}

func TestAreAnyOnline(t *testing.T) {
	// Initialize the mutexes
	lastPresenceStateMutex = sync.RWMutex{}
	anyInVoiceChannelMutex = sync.RWMutex{}

	// Test cases
	tests := []struct {
		name           string
		idsToTrack     map[string]struct{}
		lastStates     map[string]discordgo.Status
		requireVc      bool
		anyInVoiceChan bool
		expected       bool
	}{
		{
			name: "UserOnline",
			idsToTrack: map[string]struct{}{
				"user1": {},
			},
			lastStates: map[string]discordgo.Status{
				"user1": discordgo.StatusOnline,
			},
			requireVc:      false,
			anyInVoiceChan: false,
			expected:       true,
		},
		{
			name: "UserOffline",
			idsToTrack: map[string]struct{}{
				"user1": {},
			},
			lastStates: map[string]discordgo.Status{
				"user1": discordgo.StatusOffline,
			},
			requireVc:      false,
			anyInVoiceChan: false,
			expected:       false,
		},
		{
			name: "VcButNotOnline",
			idsToTrack: map[string]struct{}{
				"user1": {},
			},
			lastStates: map[string]discordgo.Status{
				"user1": discordgo.StatusOffline,
			},
			requireVc:      false,
			anyInVoiceChan: true,
			expected:       true,
		},
		{
			name: "RequireVcButNotInVoiceChannel",
			idsToTrack: map[string]struct{}{
				"user1": {},
			},
			lastStates: map[string]discordgo.Status{
				"user1": discordgo.StatusOnline,
			},
			requireVc:      true,
			anyInVoiceChan: false,
			expected:       false,
		},
		{
			name: "RequireVcAndInVoiceChannel",
			idsToTrack: map[string]struct{}{
				"user1": {},
			},
			lastStates: map[string]discordgo.Status{
				"user1": discordgo.StatusOffline,
			},
			requireVc:      true,
			anyInVoiceChan: true,
			expected:       true,
		},
		{
			name:       "NoUsersToTrack",
			idsToTrack: map[string]struct{}{
				// Empty
			},
			lastStates:     map[string]discordgo.Status{},
			requireVc:      false,
			anyInVoiceChan: false,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set global variables
			idsToTrack = tt.idsToTrack
			lastPresenceState = tt.lastStates
			*requireVc = tt.requireVc
			anyInVoiceChannel = tt.anyInVoiceChan

			result := areAnyOnline()
			if result != tt.expected {
				t.Errorf("areAnyOnline() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLoadIdsToTrack(t *testing.T) {
	originalEnv := os.Getenv("USERS_IDS_TO_TRACK")
	defer os.Setenv("USERS_IDS_TO_TRACK", originalEnv)

	tests := []struct {
		envValue string
		expected map[string]struct{}
	}{
		{
			envValue: "user1,user2,user3",
			expected: map[string]struct{}{
				"user1": {},
				"user2": {},
				"user3": {},
			},
		},
		{
			envValue: "  user1 , user2 ,user3  ",
			expected: map[string]struct{}{
				"user1": {},
				"user2": {},
				"user3": {},
			},
		},
		{
			envValue: "  user1 , ",
			expected: map[string]struct{}{
				"user1": {},
			},
		},
		{
			envValue: "user1",
			expected: map[string]struct{}{
				"user1": {},
			},
		},
		{
			envValue: "",
			expected: map[string]struct{}{},
		},
	}

	for _, tt := range tests {
		os.Setenv("USERS_IDS_TO_TRACK", tt.envValue)
		idsToTrack = make(map[string]struct{})
		loadIdsToTrack()
		if len(idsToTrack) != len(tt.expected) {
			t.Errorf("Expected %d IDs, got %d", len(tt.expected), len(idsToTrack))
		}
		for id := range tt.expected {
			if _, exists := idsToTrack[id]; !exists {
				t.Errorf("Expected ID %s to be loaded", id)
			}
		}
	}
}
