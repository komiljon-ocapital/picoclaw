// Package paths provides centralized constants for file and directory paths
// used throughout the picoclaw application.
package paths

import (
	"os"
	"path/filepath"
)

// Global config directory name (under user home)
const GlobalConfigDir = ".picoclaw"

// Workspace subdirectory names
const (
	MemoryDir   = "memory"
	SkillsDir   = "skills"
	SessionsDir = "sessions"
	CronDir     = "cron"
)

// File names
const (
	MemoryFile     = "MEMORY.md"
	HeartbeatFile  = "HEARTBEAT.md"
	SkillFile      = "SKILL.md"
	CronJobsFile   = "jobs.json"
	HeartbeatLog   = "heartbeat.log"
	HistoryFile    = ".picoclaw_history"
	ConfigFile     = "config.json"
)

// Temp directory names
const (
	MediaTempDir = "picoclaw_media"
)

// GetGlobalConfigPath returns the path to the global config directory (~/.picoclaw)
func GetGlobalConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, GlobalConfigDir)
}

// GetConfigFilePath returns the path to the config file (~/.picoclaw/config.json)
func GetConfigFilePath() string {
	return filepath.Join(GetGlobalConfigPath(), ConfigFile)
}

// GetGlobalSkillsPath returns the path to global skills directory (~/.picoclaw/skills)
func GetGlobalSkillsPath() string {
	return filepath.Join(GetGlobalConfigPath(), SkillsDir)
}

// GetBuiltinSkillsPath returns the path to builtin skills directory (~/.picoclaw/picoclaw/skills)
// This is used for skills that come bundled with picoclaw
func GetBuiltinSkillsPath() string {
	return filepath.Join(GetGlobalConfigPath(), "picoclaw", SkillsDir)
}

// GetMemoryPath returns the path to the memory directory within a workspace
func GetMemoryPath(workspace string) string {
	return filepath.Join(workspace, MemoryDir)
}

// GetMemoryFilePath returns the path to MEMORY.md within a workspace
func GetMemoryFilePath(workspace string) string {
	return filepath.Join(GetMemoryPath(workspace), MemoryFile)
}

// GetHeartbeatFilePath returns the path to HEARTBEAT.md within a workspace
func GetHeartbeatFilePath(workspace string) string {
	return filepath.Join(GetMemoryPath(workspace), HeartbeatFile)
}

// GetHeartbeatLogPath returns the path to heartbeat.log within a workspace
func GetHeartbeatLogPath(workspace string) string {
	return filepath.Join(GetMemoryPath(workspace), HeartbeatLog)
}

// GetSkillsPath returns the path to the skills directory within a workspace
func GetSkillsPath(workspace string) string {
	return filepath.Join(workspace, SkillsDir)
}

// GetSkillFilePath returns the path to a specific skill's SKILL.md file
func GetSkillFilePath(skillsDir, skillName string) string {
	return filepath.Join(skillsDir, skillName, SkillFile)
}

// GetSessionsPath returns the path to the sessions directory within a workspace
func GetSessionsPath(workspace string) string {
	return filepath.Join(workspace, SessionsDir)
}

// GetCronPath returns the path to the cron directory within a workspace
func GetCronPath(workspace string) string {
	return filepath.Join(workspace, CronDir)
}

// GetCronJobsPath returns the path to jobs.json within a workspace
func GetCronJobsPath(workspace string) string {
	return filepath.Join(GetCronPath(workspace), CronJobsFile)
}

// GetMediaTempPath returns the path to the media temp directory
func GetMediaTempPath() string {
	return filepath.Join(os.TempDir(), MediaTempDir)
}

// GetHistoryFilePath returns the path to the history file in temp
func GetHistoryFilePath() string {
	return filepath.Join(os.TempDir(), HistoryFile)
}

// EnsureWorkspaceDirs creates the standard workspace directories if they don't exist
func EnsureWorkspaceDirs(workspace string) error {
	dirs := []string{
		workspace,
		GetMemoryPath(workspace),
		GetSkillsPath(workspace),
		GetSessionsPath(workspace),
		GetCronPath(workspace),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}