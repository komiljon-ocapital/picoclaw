// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/cron"
	"github.com/sipeed/picoclaw/pkg/heartbeat"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/skills"
	"github.com/sipeed/picoclaw/pkg/tools"
	"github.com/sipeed/picoclaw/pkg/voice"
)

const version = "0.1.0"
const logo = "🦞"

// VerbosityLevel represents the verbosity level of the CLI
type VerbosityLevel int

const (
	// QuietLevel suppresses all output except errors
	QuietLevel VerbosityLevel = iota
	// NormalLevel is the default verbosity level
	NormalLevel
	// VerboseLevel provides detailed output
	VerboseLevel
)

var globalVerbosity logger.VerbosityLevel = logger.NormalLevel

func copyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	// Parse global flags
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-v", "--verbose":
			globalVerbosity = logger.VerboseLevel
			logger.SetVerbosity(logger.VerboseLevel)
		case "-q", "--quiet":
			globalVerbosity = logger.QuietLevel
			logger.SetVerbosity(logger.QuietLevel)
		}
	}

	// Remove global flags from args
	args = removeGlobalFlags(args)

	if len(args) == 0 {
		printHelp()
		os.Exit(1)
	}

	command := args[0]

	switch command {
	case "onboard":
		onboard()
	case "agent":
		agentCmd(args[1:])
	case "gateway":
		gatewayCmd(args[1:])
	case "status":
		statusCmd()
	case "cron":
		cronCmd(args[1:])
	case "skills":
		if len(args) < 2 {
			skillsHelp()
			return
		}

		subcommand := args[1]

		cfg, err := loadConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		workspace := cfg.WorkspacePath()
		installer := skills.NewSkillInstaller(workspace)
		// 获取全局配置目录和内置 skills 目录
		globalDir := filepath.Dir(getConfigPath())
		globalSkillsDir := filepath.Join(globalDir, "skills")
		builtinSkillsDir := filepath.Join(globalDir, "picoclaw", "skills")
		skillsLoader := skills.NewSkillsLoader(workspace, globalSkillsDir, builtinSkillsDir)

		switch subcommand {
		case "list":
			skillsListCmd(skillsLoader)
		case "install":
			skillsInstallCmd(installer)
		case "remove", "uninstall":
			if len(args) < 3 {
				fmt.Println("Usage: picoclaw skills remove <skill-name>")
				return
			}
			skillsRemoveCmd(installer, args[2])
		case "install-builtin":
			skillsInstallBuiltinCmd(workspace)
		case "list-builtin":
			skillsListBuiltinCmd()
		case "search":
			skillsSearchCmd(installer)
		case "show":
			if len(args) < 3 {
				fmt.Println("Usage: picoclaw skills show <skill-name>")
				return
			}
			skillsShowCmd(skillsLoader, args[2])
		default:
			fmt.Printf("Unknown skills command: %s\n", subcommand)
			skillsHelp()
		}
	case "version", "--version":
		fmt.Printf("%s picoclaw v%s\n", logo, version)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printHelp()
		os.Exit(1)
	}
}

func removeGlobalFlags(args []string) []string {
	result := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "-v" || args[i] == "--verbose" || args[i] == "-q" || args[i] == "--quiet" {
			continue
		}
		result = append(result, args[i])
	}
	return result
}

func printHelp() {
	if globalVerbosity != logger.QuietLevel {
		fmt.Printf("%s picoclaw - Personal AI Assistant v%s\n\n", logo, version)
		fmt.Println("Usage: picoclaw [global options] <command>")
		fmt.Println()
		fmt.Println("Global Options:")
		fmt.Println("  -v, --verbose  Enable verbose output")
		fmt.Println("  -q, --quiet    Suppress all output except errors")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  onboard     Initialize picoclaw configuration and workspace")
		fmt.Println("  agent       Interact with the agent directly")
		fmt.Println("  gateway     Start picoclaw gateway")
		fmt.Println("  status      Show picoclaw status")
		fmt.Println("  cron        Manage scheduled tasks")
		fmt.Println("  skills      Manage skills (install, list, remove)")
		fmt.Println("  version     Show version information")
		fmt.Println()
		fmt.Println("Use 'picoclaw <command> --help' for more information about a command.")
	} else {
		logger.InfoC("help", "Use -v or --verbose for more information.")
	}
}

func onboard() {
	configPath := getConfigPath()

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config already exists at %s\n", configPath)
		fmt.Print("Overwrite? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" {
			fmt.Println("Aborted.")
			return
		}
	}

	cfg := config.DefaultConfig()
	if err := config.SaveConfig(configPath, cfg); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	workspace := cfg.WorkspacePath()
	os.MkdirAll(workspace, 0755)
	os.MkdirAll(filepath.Join(workspace, "memory"), 0755)
	os.MkdirAll(filepath.Join(workspace, "skills"), 0755)

	createWorkspaceTemplates(workspace)

	fmt.Printf("%s picoclaw is ready!\n", logo)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Add your API key to", configPath)
	fmt.Println("     Get one at: https://openrouter.ai/keys")
	fmt.Println("  2. Chat: picoclaw agent -m \"Hello!\"")
}

func createWorkspaceTemplates(workspace string) {
	templates := map[string]string{
		"AGENTS.md": `# Agent Instructions

You are a helpful AI assistant. Be concise, accurate, and friendly.

## Guidelines

- Always explain what you're doing before taking actions
- Ask for clarification when request is ambiguous
- Use tools to help accomplish tasks
- Remember important information in your memory files
- Be proactive and helpful
- Learn from user feedback
`,
		"SOUL.md": `# Soul

I am picoclaw, a lightweight AI assistant powered by AI.

## Personality

- Helpful and friendly
- Concise and to the point
- Curious and eager to learn
- Honest and transparent

## Values

- Accuracy over speed
- User privacy and safety
- Transparency in actions
- Continuous improvement
`,
		"USER.md": `# User

Information about user goes here.

## Preferences

- Communication style: (casual/formal)
- Timezone: (your timezone)
- Language: (your preferred language)

## Personal Information

- Name: (optional)
- Location: (optional)
- Occupation: (optional)

## Learning Goals

- What the user wants to learn from AI
- Preferred interaction style
- Areas of interest
`,
		"IDENTITY.md": `# Identity

## Name
PicoClaw 🦞

## Description
Ultra-lightweight personal AI assistant written in Go, inspired by nanobot.

## Version
0.1.0

## Purpose
- Provide intelligent AI assistance with minimal resource usage
- Support multiple LLM providers (OpenAI, Anthropic, Zhipu, etc.)
- Enable easy customization through skills system
- Run on minimal hardware ($10 boards, <10MB RAM)

## Capabilities

- Web search and content fetching
- File system operations (read, write, edit)
- Shell command execution
- Multi-channel messaging (Telegram, WhatsApp, Feishu)
- Skill-based extensibility
- Memory and context management

## Philosophy

- Simplicity over complexity
- Performance over features
- User control and privacy
- Transparent operation
- Community-driven development

## Goals

- Provide a fast, lightweight AI assistant
- Support offline-first operation where possible
- Enable easy customization and extension
- Maintain high quality responses
- Run efficiently on constrained hardware

## License
MIT License - Free and open source

## Repository
https://github.com/sipeed/picoclaw

## Contact
Issues: https://github.com/sipeed/picoclaw/issues
Discussions: https://github.com/sipeed/picoclaw/discussions

---

"Every bit helps, every bit matters."
- Picoclaw
`,
	}

	for filename, content := range templates {
		filePath := filepath.Join(workspace, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			os.WriteFile(filePath, []byte(content), 0644)
			fmt.Printf("  Created %s\n", filename)
		}
	}

	memoryDir := filepath.Join(workspace, "memory")
	os.MkdirAll(memoryDir, 0755)
	memoryFile := filepath.Join(memoryDir, "MEMORY.md")
	if _, err := os.Stat(memoryFile); os.IsNotExist(err) {
		memoryContent := `# Long-term Memory

This file stores important information that should persist across sessions.

## User Information

(Important facts about user)

## Preferences

(User preferences learned over time)

## Important Notes

(Things to remember)

## Configuration

- Model preferences
- Channel settings
- Skills enabled
`
		os.WriteFile(memoryFile, []byte(memoryContent), 0644)
		fmt.Println("  Created memory/MEMORY.md")

		skillsDir := filepath.Join(workspace, "skills")
		if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
			os.MkdirAll(skillsDir, 0755)
			fmt.Println("  Created skills/")
		}
	}

	for filename, content := range templates {
		filePath := filepath.Join(workspace, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			os.WriteFile(filePath, []byte(content), 0644)
			fmt.Printf("  Created %s\n", filename)
		}
	}
}

func agentCmd(args []string) {
	message := ""
	sessionKey := "cli:default"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--debug", "-d":
			logger.SetLevel(logger.DEBUG)
			if globalVerbosity != QuietLevel {
				fmt.Println("🔍 Debug mode enabled")
			}
		case "-m", "--message":
			if i+1 < len(args) {
				message = args[i+1]
				i++
			}
		case "-s", "--session":
			if i+1 < len(args) {
				sessionKey = args[i+1]
				i++
			}
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	provider, err := providers.CreateProvider(cfg)
	if err != nil {
		fmt.Printf("Error creating provider: %v\n", err)
		os.Exit(1)
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)

	// Print agent startup info (only for interactive mode and not in quiet mode)
	if globalVerbosity != QuietLevel {
		startupInfo := agentLoop.GetStartupInfo()
		logger.InfoCF("agent", "Agent initialized",
			map[string]interface{}{
				"tools_count":      startupInfo["tools"].(map[string]interface{})["count"],
				"skills_total":     startupInfo["skills"].(map[string]interface{})["total"],
				"skills_available": startupInfo["skills"].(map[string]interface{})["available"],
			})
	}

	if message != "" {
		ctx := context.Background()
		response, err := agentLoop.ProcessDirect(ctx, message, sessionKey)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		if globalVerbosity != QuietLevel {
			fmt.Printf("\n%s %s\n", logo, response)
		}
	} else {
		if globalVerbosity != QuietLevel {
			fmt.Printf("%s Interactive mode (Ctrl+C to exit)\n\n", logo)
		}
		interactiveMode(agentLoop, sessionKey)
	}
}

func interactiveMode(agentLoop *agent.AgentLoop, sessionKey string) {
	prompt := fmt.Sprintf("%s You: ", logo)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          prompt,
		HistoryFile:     filepath.Join(os.TempDir(), ".picoclaw_history"),
		HistoryLimit:    100,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})

	if err != nil {
		if globalVerbosity != QuietLevel {
			fmt.Printf("Error initializing readline: %v\n", err)
			fmt.Println("Falling back to simple input mode...")
		}
		simpleInteractiveMode(agentLoop, sessionKey)
		return
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt || err == io.EOF {
				if globalVerbosity != QuietLevel {
					fmt.Println("\nGoodbye!")
				}
				return
			}
			if globalVerbosity != QuietLevel {
				fmt.Printf("Error reading input: %v\n", err)
			}
			continue
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			if globalVerbosity != QuietLevel {
				fmt.Println("Goodbye!")
			}
			return
		}

		ctx := context.Background()
		response, err := agentLoop.ProcessDirect(ctx, input, sessionKey)
		if err != nil {
			if globalVerbosity != QuietLevel {
				fmt.Printf("Error: %v\n", err)
			}
			continue
		}

		if globalVerbosity != QuietLevel {
			fmt.Printf("\n%s %s\n\n", logo, response)
		}
	}
}

func simpleInteractiveMode(agentLoop *agent.AgentLoop, sessionKey string) {
	reader := bufio.NewReader(os.Stdin)
	for {
		if globalVerbosity != QuietLevel {
			fmt.Print(fmt.Sprintf("%s You: ", logo))
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if globalVerbosity != QuietLevel {
					fmt.Println("\nGoodbye!")
				}
				return
			}
			if globalVerbosity != QuietLevel {
				fmt.Printf("Error reading input: %v\n", err)
			}
			continue
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			if globalVerbosity != QuietLevel {
				fmt.Println("Goodbye!")
			}
			return
		}

		ctx := context.Background()
		response, err := agentLoop.ProcessDirect(ctx, input, sessionKey)
		if err != nil {
			if globalVerbosity != QuietLevel {
				fmt.Printf("Error: %v\n", err)
			}
			continue
		}

		if globalVerbosity != QuietLevel {
			fmt.Printf("\n%s %s\n\n", logo, response)
		}
	}
}

func gatewayCmd(args []string) {
	// Check for --debug flag
	for _, arg := range args {
		if arg == "--debug" || arg == "-d" {
			logger.SetLevel(logger.DEBUG)
			if globalVerbosity != QuietLevel {
				fmt.Println("🔍 Debug mode enabled")
			}
			break
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	provider, err := providers.CreateProvider(cfg)
	if err != nil {
		fmt.Printf("Error creating provider: %v\n", err)
		os.Exit(1)
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)

	// Print agent startup info
	if globalVerbosity != QuietLevel {
		fmt.Println("\n📦 Agent Status:")
		startupInfo := agentLoop.GetStartupInfo()
		toolsInfo := startupInfo["tools"].(map[string]interface{})
		skillsInfo := startupInfo["skills"].(map[string]interface{})
		fmt.Printf("  • Tools: %d loaded\n", toolsInfo["count"])
		fmt.Printf("  • Skills: %d/%d available\n",
			skillsInfo["available"],
			skillsInfo["total"])
	}

	// Log to file as well
	startupInfo := agentLoop.GetStartupInfo()
	toolsInfo := startupInfo["tools"].(map[string]interface{})
	skillsInfo := startupInfo["skills"].(map[string]interface{})
	logger.InfoCF("agent", "Agent initialized",
		map[string]interface{}{
			"tools_count":      toolsInfo["count"],
			"skills_total":     skillsInfo["total"],
			"skills_available": skillsInfo["available"],
		})

	// Setup cron tool and service
	cronService := setupCronTool(agentLoop, msgBus, cfg.WorkspacePath())

	heartbeatService := heartbeat.NewHeartbeatService(
		cfg.WorkspacePath(),
		nil,
		30*60,
		true,
	)

	channelManager, err := channels.NewManager(cfg, msgBus)
	if err != nil {
		fmt.Printf("Error creating channel manager: %v\n", err)
		os.Exit(1)
	}

	var transcriber *voice.GroqTranscriber
	if cfg.Providers.Groq.APIKey != "" {
		transcriber = voice.NewGroqTranscriber(cfg.Providers.Groq.APIKey)
		logger.InfoC("voice", "Groq voice transcription enabled")
	}

	if transcriber != nil {
		if telegramChannel, ok := channelManager.GetChannel("telegram"); ok {
			if tc, ok := telegramChannel.(*channels.TelegramChannel); ok {
				tc.SetTranscriber(transcriber)
				logger.InfoC("voice", "Groq transcription attached to Telegram channel")
			}
		}
		if discordChannel, ok := channelManager.GetChannel("discord"); ok {
			if dc, ok := discordChannel.(*channels.DiscordChannel); ok {
				dc.SetTranscriber(transcriber)
				logger.InfoC("voice", "Groq transcription attached to Discord channel")
			}
		}
	}

	enabledChannels := channelManager.GetEnabledChannels()
	if globalVerbosity != QuietLevel {
		if len(enabledChannels) > 0 {
			fmt.Printf("✓ Channels enabled: %s\n", enabledChannels)
		} else {
			fmt.Println("⚠ Warning: No channels enabled")
		}

		fmt.Printf("✓ Gateway started on %s:%d\n", cfg.Gateway.Host, cfg.Gateway.Port)
		fmt.Println("Press Ctrl+C to stop")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cronService.Start(); err != nil {
		fmt.Printf("Error starting cron service: %v\n", err)
	}
	if globalVerbosity != QuietLevel {
		fmt.Println("✓ Cron service started")
	}

	if err := heartbeatService.Start(); err != nil {
		fmt.Printf("Error starting heartbeat service: %v\n", err)
	}
	if globalVerbosity != QuietLevel {
		fmt.Println("✓ Heartbeat service started")
	}

	if err := channelManager.StartAll(ctx); err != nil {
		fmt.Printf("Error starting channels: %v\n", err)
	}

	go agentLoop.Run(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	if globalVerbosity != QuietLevel {
		fmt.Println("\nShutting down...")
	}
	cancel()
	heartbeatService.Stop()
	cronService.Stop()
	agentLoop.Stop()
	channelManager.StopAll(ctx)
	if globalVerbosity != QuietLevel {
		fmt.Println("✓ Gateway stopped")
	}
}

func statusCmd() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	configPath := getConfigPath()

	if globalVerbosity != logger.QuietLevel {
		fmt.Printf("%s picoclaw Status\n\n", logo)
	}

	if _, err := os.Stat(configPath); err == nil {
		logger.InfoC("status", fmt.Sprintf("Config: %s ✓", configPath))
	} else {
		logger.WarnC("status", fmt.Sprintf("Config: %s ✗", configPath))
	}

	workspace := cfg.WorkspacePath()
	if _, err := os.Stat(workspace); err == nil {
		logger.InfoC("status", fmt.Sprintf("Workspace: %s ✓", workspace))
	} else {
		logger.WarnC("status", fmt.Sprintf("Workspace: %s ✗", workspace))
	}

	if _, err := os.Stat(configPath); err == nil {
		logger.InfoC("status", fmt.Sprintf("Model: %s", cfg.Agents.Defaults.Model))

		hasOpenRouter := cfg.Providers.OpenRouter.APIKey != ""
		hasAnthropic := cfg.Providers.Anthropic.APIKey != ""
		hasOpenAI := cfg.Providers.OpenAI.APIKey != ""
		hasGemini := cfg.Providers.Gemini.APIKey != ""
		hasZhipu := cfg.Providers.Zhipu.APIKey != ""
		hasGroq := cfg.Providers.Groq.APIKey != ""
		hasVLLM := cfg.Providers.VLLM.APIBase != ""

		status := func(enabled bool) string {
			if enabled {
				return "✓"
			}
			return "not set"
		}
		logger.InfoC("status", fmt.Sprintf("OpenRouter API: %s", status(hasOpenRouter)))
		logger.InfoC("status", fmt.Sprintf("Anthropic API: %s", status(hasAnthropic)))
		logger.InfoC("status", fmt.Sprintf("OpenAI API: %s", status(hasOpenAI)))
		logger.InfoC("status", fmt.Sprintf("Gemini API: %s", status(hasGemini)))
		logger.InfoC("status", fmt.Sprintf("Zhipu API: %s", status(hasZhipu)))
		logger.InfoC("status", fmt.Sprintf("Groq API: %s", status(hasGroq)))
		if hasVLLM {
			logger.InfoC("status", fmt.Sprintf("vLLM/Local: ✓ %s", cfg.Providers.VLLM.APIBase))
		} else {
			logger.InfoC("status", "vLLM/Local: not set")
		}
	}
}

func getConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "config.json")
}

func setupCronTool(agentLoop *agent.AgentLoop, msgBus *bus.MessageBus, workspace string) *cron.CronService {
	cronStorePath := filepath.Join(workspace, "cron", "jobs.json")

	// Create cron service
	cronService := cron.NewCronService(cronStorePath, nil)

	// Create and register CronTool
	cronTool := tools.NewCronTool(cronService, agentLoop, msgBus)
	agentLoop.RegisterTool(cronTool)

	// Set the onJob handler
	cronService.SetOnJob(func(job *cron.CronJob) (string, error) {
		result := cronTool.ExecuteJob(context.Background(), job)
		return result, nil
	})

	return cronService
}

func loadConfig() (*config.Config, error) {
	return config.LoadConfig(getConfigPath())
}

func cronCmd(args []string) {
	if len(args) < 1 {
		cronHelp()
		return
	}

	subcommand := args[0]

	// Load config to get workspace path
	cfg, err := loadConfig()
	if err != nil {
		logger.ErrorC("cron", fmt.Sprintf("Error loading config: %v", err))
		return
	}

	cronStorePath := filepath.Join(cfg.WorkspacePath(), "cron", "jobs.json")

	switch subcommand {
	case "list":
		cronListCmd(cronStorePath)
	case "add":
		cronAddCmd(cronStorePath)
	case "remove":
		if len(args) < 2 {
			logger.ErrorC("cron", "Usage: picoclaw cron remove <job_id>")
			return
		}
		cronRemoveCmd(cronStorePath, args[1])
	case "enable":
		cronEnableCmd(cronStorePath, false)
	case "disable":
		cronEnableCmd(cronStorePath, true)
	default:
		logger.ErrorC("cron", fmt.Sprintf("Unknown cron command: %s", subcommand))
		cronHelp()
	}
}

func cronHelp() {
	if globalVerbosity != logger.QuietLevel {
		logger.InfoC("cron", "\nCron commands:")
		logger.InfoC("cron", "  list              List all scheduled jobs")
		logger.InfoC("cron", "  add               Add a new scheduled job")
		logger.InfoC("cron", "  remove <id>       Remove a job by ID")
		logger.InfoC("cron", "  enable <id>       Enable a job")
		logger.InfoC("cron", "  disable <id>      Disable a job")
		logger.InfoC("cron", "")
		logger.InfoC("cron", "Add options:")
		logger.InfoC("cron", "  -n, --name        Job name")
		logger.InfoC("cron", "  -m, --message     Message for agent")
		logger.InfoC("cron", "  -e, --every       Run every N seconds")
		logger.InfoC("cron", "  -c, --cron        Cron expression (e.g. '0 9 * * *')")
		logger.InfoC("cron", "  -d, --deliver     Deliver response to channel")
		logger.InfoC("cron", "  --to              Recipient for delivery")
		logger.InfoC("cron", "  --channel         Channel for delivery")
		logger.InfoC("cron", "")
		logger.InfoC("cron", "Use -v or --verbose for more detailed output")
		logger.InfoC("cron", "Use -q or --quiet to suppress all output except errors")
	} else {
		logger.InfoC("cron", "Use -v or --verbose for more information about cron commands.")
	}
}

func cronListCmd(storePath string) {
	cs := cron.NewCronService(storePath, nil)
	jobs := cs.ListJobs(true)  // Show all jobs, including disabled

	if len(jobs) == 0 {
		logger.InfoC("cron", "No scheduled jobs.")
		return
	}

	if globalVerbosity != logger.QuietLevel {
		logger.InfoC("cron", "Scheduled Jobs:")
		logger.InfoC("cron", "----------------")
	}

	for _, job := range jobs {
		var schedule string
		if job.Schedule.Kind == "every" && job.Schedule.EveryMS != nil {
			schedule = fmt.Sprintf("every %ds", *job.Schedule.EveryMS/1000)
		} else if job.Schedule.Kind == "cron" {
			schedule = job.Schedule.Expr
		} else {
			schedule = "one-time"
		}

		nextRun := "scheduled"
		if job.State.NextRunAtMS != nil {
			nextTime := time.UnixMilli(*job.State.NextRunAtMS)
			nextRun = nextTime.Format("2006-01-02 15:04")
		}

		status := "enabled"
		if !job.Enabled {
			status = "disabled"
		}

		jobInfo := fmt.Sprintf("%s (%s)\n    Schedule: %s\n    Status: %s\n    Next run: %s",
			job.Name, job.ID, schedule, status, nextRun)
		logger.InfoC("cron", jobInfo)
	}
}

func cronAddCmd(storePath string) {
	name := ""
	message := ""
	var everySec *int64
	cronExpr := ""
	deliver := false
	channel := ""
	to := ""

	args := os.Args[3:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n", "--name":
			if i+1 < len(args) {
				name = args[i+1]
				i++
			}
		case "-m", "--message":
			if i+1 < len(args) {
				message = args[i+1]
				i++
			}
		case "-e", "--every":
			if i+1 < len(args) {
				var sec int64
				fmt.Sscanf(args[i+1], "%d", &sec)
				everySec = &sec
				i++
			}
		case "-c", "--cron":
			if i+1 < len(args) {
				cronExpr = args[i+1]
				i++
			}
		case "-d", "--deliver":
			deliver = true
		case "--to":
			if i+1 < len(args) {
				to = args[i+1]
				i++
			}
		case "--channel":
			if i+1 < len(args) {
				channel = args[i+1]
				i++
			}
		}
	}

	if name == "" {
		fmt.Println("Error: --name is required")
		return
	}

	if message == "" {
		fmt.Println("Error: --message is required")
		return
	}

	if everySec == nil && cronExpr == "" {
		fmt.Println("Error: Either --every or --cron must be specified")
		return
	}

	var schedule cron.CronSchedule
	if everySec != nil {
		everyMS := *everySec * 1000
		schedule = cron.CronSchedule{
			Kind:    "every",
			EveryMS: &everyMS,
		}
	} else {
		schedule = cron.CronSchedule{
			Kind: "cron",
			Expr: cronExpr,
		}
	}

	cs := cron.NewCronService(storePath, nil)
	job, err := cs.AddJob(name, schedule, message, deliver, channel, to)
	if err != nil {
		fmt.Printf("Error adding job: %v\n", err)
		return
	}

	fmt.Printf("✓ Added job '%s' (%s)\n", job.Name, job.ID)
}

func cronRemoveCmd(storePath, jobID string) {
	cs := cron.NewCronService(storePath, nil)
	if cs.RemoveJob(jobID) {
		fmt.Printf("✓ Removed job %s\n", jobID)
	} else {
		fmt.Printf("✗ Job %s not found\n", jobID)
	}
}

func cronEnableCmd(storePath string, disable bool) {
	if len(os.Args) < 4 {
		fmt.Println("Usage: picoclaw cron enable/disable <job_id>")
		return
	}

	jobID := os.Args[3]
	cs := cron.NewCronService(storePath, nil)
	enabled := !disable

	job := cs.EnableJob(jobID, enabled)
	if job != nil {
		status := "enabled"
		if disable {
			status = "disabled"
		}
		fmt.Printf("✓ Job '%s' %s\n", job.Name, status)
	} else {
		fmt.Printf("✗ Job %s not found\n", jobID)
	}
}

func skillsCmd(args []string) {
	if len(args) < 1 {
		skillsHelp()
		return
	}

	subcommand := args[0]

	cfg, err := loadConfig()
	if err != nil {
		logger.ErrorC("skills", fmt.Sprintf("Error loading config: %v", err))
		os.Exit(1)
	}

	workspace := cfg.WorkspacePath()
	installer := skills.NewSkillInstaller(workspace)
	// 获取全局配置目录和内置 skills 目录
	globalDir := filepath.Dir(getConfigPath())
	globalSkillsDir := filepath.Join(globalDir, "skills")
	builtinSkillsDir := filepath.Join(globalDir, "picoclaw", "skills")
	skillsLoader := skills.NewSkillsLoader(workspace, globalSkillsDir, builtinSkillsDir)

	switch subcommand {
	case "list":
		skillsListCmd(skillsLoader)
	case "install":
		skillsInstallCmd(installer)
	case "remove", "uninstall":
		if len(args) < 2 {
			logger.ErrorC("skills", "Usage: picoclaw skills remove <skill-name>")
			return
		}
		skillsRemoveCmd(installer, args[1])
	case "search":
		skillsSearchCmd(installer)
	case "show":
		if len(args) < 2 {
			logger.ErrorC("skills", "Usage: picoclaw skills show <skill-name>")
			return
		}
		skillsShowCmd(skillsLoader, args[1])
	default:
		logger.ErrorC("skills", fmt.Sprintf("Unknown skills command: %s", subcommand))
		skillsHelp()
	}
}

func skillsHelp() {
	if globalVerbosity != logger.QuietLevel {
		logger.InfoC("skills", "\nSkills commands:")
		logger.InfoC("skills", "  list                    List installed skills")
		logger.InfoC("skills", "  install <repo>          Install skill from GitHub")
		logger.InfoC("skills", "  install-builtin         Install all builtin skills to workspace")
		logger.InfoC("skills", "  list-builtin            List available builtin skills")
		logger.InfoC("skills", "  remove <n>              Remove installed skill")
		logger.InfoC("skills", "  search                  Search available skills")
		logger.InfoC("skills", "  show <n>                Show skill details")
		logger.InfoC("skills", "")
		logger.InfoC("skills", "Examples:")
		logger.InfoC("skills", "  picoclaw skills list")
		logger.InfoC("skills", "  picoclaw skills install sipeed/picoclaw-skills/weather")
		logger.InfoC("skills", "  picoclaw skills install-builtin")
		logger.InfoC("skills", "  picoclaw skills list-builtin")
		logger.InfoC("skills", "  picoclaw skills remove weather")
		logger.InfoC("skills", "")
		logger.InfoC("skills", "Use -v or --verbose for more detailed output")
		logger.InfoC("skills", "Use -q or --quiet to suppress all output except errors")
	} else {
		logger.InfoC("skills", "Use -v or --verbose for more information about skills commands.")
	}
}

func skillsListCmd(loader *skills.SkillsLoader) {
	allSkills := loader.ListSkills()

	if len(allSkills) == 0 {
		logger.InfoC("skills", "No skills installed.")
		return
	}

	if globalVerbosity != logger.QuietLevel {
		logger.InfoC("skills", "Installed Skills:")
		logger.InfoC("skills", "------------------")
	}

	for _, skill := range allSkills {
		skillInfo := fmt.Sprintf("✓ %s (%s)", skill.Name, skill.Source)
		if skill.Description != "" {
			skillInfo += fmt.Sprintf("\n    %s", skill.Description)
		}
		logger.InfoC("skills", skillInfo)
	}
}

func skillsInstallCmd(installer *skills.SkillInstaller) {
	if len(os.Args) < 4 {
		fmt.Println("Usage: picoclaw skills install <github-repo>")
		fmt.Println("Example: picoclaw skills install sipeed/picoclaw-skills/weather")
		return
	}

	repo := os.Args[3]
	fmt.Printf("Installing skill from %s...\n", repo)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := installer.InstallFromGitHub(ctx, repo); err != nil {
		fmt.Printf("✗ Failed to install skill: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Skill '%s' installed successfully!\n", filepath.Base(repo))
}

func skillsRemoveCmd(installer *skills.SkillInstaller, skillName string) {
	fmt.Printf("Removing skill '%s'...\n", skillName)

	if err := installer.Uninstall(skillName); err != nil {
		fmt.Printf("✗ Failed to remove skill: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Skill '%s' removed successfully!\n", skillName)
}

func skillsInstallBuiltinCmd(workspace string) {
	builtinSkillsDir := "./picoclaw/skills"
	workspaceSkillsDir := filepath.Join(workspace, "skills")

	fmt.Printf("Copying builtin skills to workspace...\n")

	skillsToInstall := []string{
		"weather",
		"news",
		"stock",
		"calculator",
	}

	for _, skillName := range skillsToInstall {
		builtinPath := filepath.Join(builtinSkillsDir, skillName)
		workspacePath := filepath.Join(workspaceSkillsDir, skillName)

		if _, err := os.Stat(builtinPath); err != nil {
			fmt.Printf("⊘ Builtin skill '%s' not found: %v\n", skillName, err)
			continue
		}

		if err := os.MkdirAll(workspacePath, 0755); err != nil {
			fmt.Printf("✗ Failed to create directory for %s: %v\n", skillName, err)
			continue
		}

		if err := copyDirectory(builtinPath, workspacePath); err != nil {
			fmt.Printf("✗ Failed to copy %s: %v\n", skillName, err)
		}
	}

	fmt.Println("\n✓ All builtin skills installed!")
	fmt.Println("Now you can use them in your workspace.")
}

func skillsListBuiltinCmd() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}
	builtinSkillsDir := filepath.Join(filepath.Dir(cfg.WorkspacePath()), "picoclaw", "skills")

	fmt.Println("\nAvailable Builtin Skills:")
	fmt.Println("-----------------------")

	entries, err := os.ReadDir(builtinSkillsDir)
	if err != nil {
		fmt.Printf("Error reading builtin skills: %v\n", err)
		return
	}

	if len(entries) == 0 {
		fmt.Println("No builtin skills available.")
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			skillName := entry.Name()
			skillFile := filepath.Join(builtinSkillsDir, skillName, "SKILL.md")

			description := "No description"
			if _, err := os.Stat(skillFile); err == nil {
				data, err := os.ReadFile(skillFile)
				if err == nil {
					content := string(data)
					if idx := strings.Index(content, "\n"); idx > 0 {
						firstLine := content[:idx]
						if strings.Contains(firstLine, "description:") {
							descLine := strings.Index(content[idx:], "\n")
							if descLine > 0 {
								description = strings.TrimSpace(content[idx+descLine : idx+descLine])
							}
						}
					}
				}
			}
			status := "✓"
			fmt.Printf("  %s  %s\n", status, entry.Name())
			if description != "" {
				fmt.Printf("     %s\n", description)
			}
		}
	}
}

func skillsSearchCmd(installer *skills.SkillInstaller) {
	fmt.Println("Searching for available skills...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	availableSkills, err := installer.ListAvailableSkills(ctx)
	if err != nil {
		fmt.Printf("✗ Failed to fetch skills list: %v\n", err)
		return
	}

	if len(availableSkills) == 0 {
		fmt.Println("No skills available.")
		return
	}

	fmt.Printf("\nAvailable Skills (%d):\n", len(availableSkills))
	fmt.Println("--------------------")
	for _, skill := range availableSkills {
		fmt.Printf("  📦 %s\n", skill.Name)
		fmt.Printf("     %s\n", skill.Description)
		fmt.Printf("     Repo: %s\n", skill.Repository)
		if skill.Author != "" {
			fmt.Printf("     Author: %s\n", skill.Author)
		}
		if len(skill.Tags) > 0 {
			fmt.Printf("     Tags: %v\n", skill.Tags)
		}
		fmt.Println()
	}
}

func skillsShowCmd(loader *skills.SkillsLoader, skillName string) {
	content, ok := loader.LoadSkill(skillName)
	if !ok {
		fmt.Printf("✗ Skill '%s' not found\n", skillName)
		return
	}

	fmt.Printf("\n📦 Skill: %s\n", skillName)
	fmt.Println("----------------------")
	fmt.Println(content)
}
