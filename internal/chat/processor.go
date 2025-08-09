package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"slbot/internal/config"
	"slbot/internal/corrade"
	"slbot/internal/macros"
	"slbot/internal/types"
)

// Processor handles chat processing and AI responses
type Processor struct {
	config               *config.Config
	corradeClient        *corrade.Client
	macroManager         *macros.Manager
	httpClient           *http.Client
	followTarget         *types.FollowTarget
	isFollowing          bool
	logs                 []types.LogEntry
	logsMutex            sync.RWMutex
	llamaEnabled         bool
	lastInteractionTime  time.Time
	idleBehaviorRunning  bool
	idleBehaviorStopChan chan struct{}
	pendingSitRequest    *types.PendingSitConfirmation
	sitRequestMutex      sync.Mutex
}

// NewProcessor creates a new chat processor
func NewProcessor(cfg *config.Config, corradeClient *corrade.Client) *Processor {
	processor := &Processor{
		config:               cfg,
		corradeClient:        corradeClient,
		httpClient:           &http.Client{Timeout: time.Duration(cfg.Bot.ResponseTimeout) * time.Second},
		isFollowing:          false,
		logs:                 make([]types.LogEntry, 0, 1000),
		llamaEnabled:         cfg.Llama.Enabled,
		lastInteractionTime:  time.Now(),
		idleBehaviorRunning:  false,
		idleBehaviorStopChan: make(chan struct{}),
	}

	// Initialize macro manager
	processor.macroManager = macros.NewManager(cfg, corradeClient)

	// Set the bot name in the corrade client for position queries
	processor.corradeClient.SetBotName(cfg.Bot.Name)

	return processor
}

// TestConnection tests the connection to Llama (if enabled)
func (p *Processor) TestConnection() error {
	if !p.llamaEnabled {
		log.Println("Llama chat is disabled - bot will use fallback responses")
		return nil
	}
	
	_, err := p.getLlamaResponse("Hello, are you working?", "chat")
	if err != nil {
		log.Printf("Llama connection failed, disabling AI chat: %v", err)
		p.llamaEnabled = false
		return nil // Don't fail startup, just disable AI
	}
	
	log.Println("Llama connection successful")
	return nil
}

// Start starts the chat processor
func (p *Processor) Start(ctx context.Context) error {
	// Set up notifications for chat events instead of polling
	if err := p.setupNotifications(); err != nil {
		log.Printf("Failed to setup notifications: %v", err)
		return err
	}

	// Start follow routine
	go p.followRoutine(ctx)

	// Start idle behavior routine
	go p.idleBehaviorRoutine(ctx)

	// Keep the context alive
	<-ctx.Done()
	return nil
}

// setupNotifications sets up Corrade notifications for chat events
func (p *Processor) setupNotifications() error {
	// Set up notification for LocalChat
	err := p.corradeClient.SetupNotification("LocalChat", fmt.Sprintf("http://localhost:%d/corrade/notifications", p.config.Bot.WebPort))
	if err != nil {
		log.Printf("Failed to setup LocalChat notification: %v", err)
	}

	// Set up notification for InstantMessage
	err = p.corradeClient.SetupNotification("InstantMessage", fmt.Sprintf("http://localhost:%d/corrade/notifications", p.config.Bot.WebPort))
	if err != nil {
		log.Printf("Failed to setup InstantMessage notification: %v", err)
	}

	return nil
}

// HandleNotification processes incoming notifications from Corrade
func (p *Processor) HandleNotification(notification map[string]interface{}) {
	// Extract event type
	eventType, ok := notification["Type"].(string)
	if !ok {
		return
	}

	// Process LocalChat and InstantMessage events
	if eventType == "LocalChat" || eventType == "InstantMessage" {
		// Extract message data
		avatar, _ := notification["FirstName"].(string)
		lastName, _ := notification["LastName"].(string)
		if lastName != "" {
			avatar += " " + lastName
		}
		message, _ := notification["Message"].(string)

		if avatar != "" && message != "" {
			chatMessage := types.ChatMessage{
				Avatar:  avatar,
				Message: message,
				Type:    eventType,
			}

			go p.processChat(chatMessage)
		}
	}
}

// processChat processes incoming chat messages
func (p *Processor) processChat(message types.ChatMessage) {
	// Update last interaction time
	p.lastInteractionTime = time.Now()

	// Skip if message is from the bot itself (avoid loops)
	if strings.Contains(message.Type, "self") {
		return
	}

	// Check if bot is mentioned or being directly addressed
	if !strings.Contains(strings.ToLower(message.Message), strings.ToLower(p.config.Bot.Name)) &&
		!strings.HasPrefix(message.Message, "/") {
		return
	}

	// Handle movement commands
	if p.handleMovementCommands(message) {
		return
	}

	// Handle macro commands
	if p.handleMacroCommands(message) {
		return
	}

	// Clean the message for processing
	cleanMessage := strings.ReplaceAll(message.Message, p.config.Bot.Name, "")
	cleanMessage = strings.TrimSpace(cleanMessage)

	// Determine context based on message content
	context := "general"
	if strings.Contains(strings.ToLower(cleanMessage), "hello") ||
		strings.Contains(strings.ToLower(cleanMessage), "hi") ||
		strings.Contains(strings.ToLower(cleanMessage), "hey") {
		context = "greeting"
	} else if strings.Contains(strings.ToLower(cleanMessage), "help") {
		context = "help"
	}

	var response string
	var err error

	// Get response from Llama if enabled, otherwise use fallbacks
	if p.llamaEnabled {
		response, err = p.getLlamaResponse(cleanMessage, context)
		if err != nil {
			log.Printf("Error getting Llama response: %v", err)
			// Fall back to predefined responses if Llama fails
			response = p.getFallbackResponse(context, cleanMessage)
		}
	} else {
		response = p.getFallbackResponse(context, cleanMessage)
	}

	// Truncate response if too long for SL chat
	if len(response) > p.config.Bot.MaxMessageLen {
		response = response[:p.config.Bot.MaxMessageLen-3] + "..."
	}

	// Send response back to Second Life
	if err := p.corradeClient.Say(response); err != nil {
		log.Printf("Error sending response to SL: %v", err)
	}

	log.Printf("Chat - %s: %s | Bot: %s", message.Avatar, message.Message, response)

	// Log to web interface
	p.addLog(types.LogEntry{
		Timestamp: time.Now(),
		Type:      "chat",
		Avatar:    message.Avatar,
		Message:   message.Message,
		Response:  response,
	})
}

// handleMovementCommands processes movement and sitting commands
func (p *Processor) handleMovementCommands(message types.ChatMessage) bool {
	msg := strings.ToLower(message.Message)

	// Follow commands
	if strings.Contains(msg, "follow me") || strings.Contains(msg, "come here") {
		err := p.followAvatar(message.Avatar)
		if err != nil {
			p.corradeClient.Say("Sorry, I can't follow you right now.")
			log.Printf("Follow error: %v", err)
		} else {
			p.corradeClient.Say(fmt.Sprintf("Following %s!", message.Avatar))
			p.addLog(types.LogEntry{
				Timestamp: time.Now(),
				Type:      "movement",
				Avatar:    message.Avatar,
				Message:   fmt.Sprintf("Started following %s", message.Avatar),
			})
			
			// Record action if recording
			p.recordAction("follow", map[string]interface{}{
				"avatar": message.Avatar,
			})
		}
		return true
	}

	// Stop following
	if strings.Contains(msg, "stop following") || strings.Contains(msg, "stay here") {
		p.stopFollowing()
		p.corradeClient.Say("I've stopped following.")
		p.recordAction("stop_follow", map[string]interface{}{})
		return true
	}

	// Sit commands
	if strings.HasPrefix(msg, "sit on ") {
		objectName := strings.TrimPrefix(msg, "sit on ")
		objectName = strings.TrimSpace(objectName)
		
		err := p.handleSitCommand(objectName, message.Avatar)
		if err != nil {
			log.Printf("Sit error: %v", err)
		}
		return true
	}

	// Handle sit confirmations - but since we removed the complex sit logic,
	// we don't need this anymore, so just return false
	// if p.handleSitConfirmation(message) {
	//	return true
	// }

	// Stand up commands
	if strings.Contains(msg, "stand up") || strings.Contains(msg, "get up") {
		status := p.corradeClient.GetStatus()
		if status.IsSitting {
			err := p.corradeClient.StandUp()
			if err != nil {
				p.corradeClient.Say("I'm having trouble standing up.")
				log.Printf("Stand error: %v", err)
			} else {
				p.corradeClient.Say("Standing up!")
				p.recordAction("stand", map[string]interface{}{})
			}
		} else {
			p.corradeClient.Say("I'm already standing.")
		}
		return true
	}

	// Move to coordinates (e.g., "go to 128 128 22")
	coordRegex := regexp.MustCompile(`go to (\d+(?:\.\d+)?) (\d+(?:\.\d+)?) (\d+(?:\.\d+)?)`)
	matches := coordRegex.FindStringSubmatch(msg)
	if len(matches) == 4 {
		var x, y, z float64
		fmt.Sscanf(matches[1], "%f", &x)
		fmt.Sscanf(matches[2], "%f", &y)
		fmt.Sscanf(matches[3], "%f", &z)

		err := p.corradeClient.WalkTo(x, y, z)
		if err != nil {
			p.corradeClient.Say("I can't reach that location.")
			log.Printf("Walk error: %v", err)
		} else {
			p.corradeClient.Say(fmt.Sprintf("Moving to %.0f, %.0f, %.0f", x, y, z))
			p.recordAction("walk", map[string]interface{}{
				"x": x,
				"y": y,
				"z": z,
			})
		}
		return true
	}

	return false
}

// handleMacroCommands processes macro recording and playback commands
func (p *Processor) handleMacroCommands(message types.ChatMessage) bool {
	msg := strings.ToLower(message.Message)
	
	// Check if user is an owner
	if !p.macroManager.IsOwner(message.Avatar) {
		return false
	}

	// Start recording macro
	if strings.HasPrefix(msg, "record macro ") {
		macroName := strings.TrimPrefix(message.Message, "record macro ")
		macroName = strings.TrimPrefix(macroName, "Record macro ")
		macroName = strings.TrimSpace(macroName)
		
		err := p.macroManager.StartRecording(macroName, message.Avatar)
		if err != nil {
			p.corradeClient.Say(fmt.Sprintf("Cannot start recording: %s", err.Error()))
		} else {
			p.corradeClient.Say(fmt.Sprintf("Started recording macro '%s'. Perform actions then say 'stop recording'.", macroName))
		}
		return true
	}

	// Stop recording macro
	if strings.Contains(msg, "stop recording") {
		// Extract description and tags if provided
		description := ""
		tags := []string{}
		isIdleBehavior := false
		
		// Parse extended stop recording syntax
		parts := strings.Split(message.Message, " ")
		for i, part := range parts {
			if strings.EqualFold(part, "description") && i+1 < len(parts) {
				description = strings.Join(parts[i+1:], " ")
				break
			}
			if strings.EqualFold(part, "tags") && i+1 < len(parts) {
				tagsPart := parts[i+1]
				if strings.Contains(tagsPart, ",") {
					tags = strings.Split(tagsPart, ",")
				} else {
					tags = []string{tagsPart}
				}
				// Clean up tags
				for j := range tags {
					tags[j] = strings.TrimSpace(tags[j])
				}
			}
			if strings.EqualFold(part, "idle") {
				isIdleBehavior = true
			}
		}
		
		err := p.macroManager.StopRecording(description, tags, isIdleBehavior)
		if err != nil {
			p.corradeClient.Say(fmt.Sprintf("Cannot stop recording: %s", err.Error()))
		} else {
			response := "Recording stopped and macro saved!"
			if isIdleBehavior {
				response += " (marked as idle behavior)"
			}
			p.corradeClient.Say(response)
		}
		return true
	}

	// Cancel recording
	if strings.Contains(msg, "cancel recording") {
		err := p.macroManager.CancelRecording()
		if err != nil {
			p.corradeClient.Say(fmt.Sprintf("Cannot cancel recording: %s", err.Error()))
		} else {
			p.corradeClient.Say("Recording cancelled.")
		}
		return true
	}

	// Play macro
	if strings.HasPrefix(msg, "play macro ") {
		macroName := strings.TrimPrefix(message.Message, "play macro ")
		macroName = strings.TrimPrefix(macroName, "Play macro ")
		macroName = strings.TrimSpace(macroName)
		
		err := p.macroManager.PlayMacro(macroName, message.Avatar)
		if err != nil {
			p.corradeClient.Say(fmt.Sprintf("Cannot play macro: %s", err.Error()))
		} else {
			p.corradeClient.Say(fmt.Sprintf("Playing macro '%s'...", macroName))
		}
		return true
	}

	// List macros
	if strings.Contains(msg, "list macros") {
		macros := p.macroManager.GetMacros()
		if len(macros) == 0 {
			p.corradeClient.Say("No macros available.")
		} else {
			macroNames := make([]string, 0, len(macros))
			for name := range macros {
				macroNames = append(macroNames, name)
			}
			p.corradeClient.Say(fmt.Sprintf("Available macros: %s", strings.Join(macroNames, ", ")))
		}
		return true
	}

	// Delete macro
	if strings.HasPrefix(msg, "delete macro ") {
		macroName := strings.TrimPrefix(message.Message, "delete macro ")
		macroName = strings.TrimPrefix(macroName, "Delete macro ")
		macroName = strings.TrimSpace(macroName)
		
		err := p.macroManager.DeleteMacro(macroName, message.Avatar)
		if err != nil {
			p.corradeClient.Say(fmt.Sprintf("Cannot delete macro: %s", err.Error()))
		} else {
			p.corradeClient.Say(fmt.Sprintf("Deleted macro '%s'.", macroName))
		}
		return true
	}

	// Set/unset idle behavior
	if strings.HasPrefix(msg, "set idle ") {
		macroName := strings.TrimPrefix(message.Message, "set idle ")
		macroName = strings.TrimPrefix(macroName, "Set idle ")
		macroName = strings.TrimSpace(macroName)
		
		err := p.macroManager.SetIdleBehavior(macroName, message.Avatar, true)
		if err != nil {
			p.corradeClient.Say(fmt.Sprintf("Cannot set idle behavior: %s", err.Error()))
		} else {
			p.corradeClient.Say(fmt.Sprintf("Macro '%s' is now an idle behavior.", macroName))
		}
		return true
	}

	if strings.HasPrefix(msg, "unset idle ") {
		macroName := strings.TrimPrefix(message.Message, "unset idle ")
		macroName = strings.TrimPrefix(macroName, "Unset idle ")
		macroName = strings.TrimSpace(macroName)
		
		err := p.macroManager.SetIdleBehavior(macroName, message.Avatar, false)
		if err != nil {
			p.corradeClient.Say(fmt.Sprintf("Cannot unset idle behavior: %s", err.Error()))
		} else {
			p.corradeClient.Say(fmt.Sprintf("Macro '%s' is no longer an idle behavior.", macroName))
		}
		return true
	}

	// List idle behaviors
	if strings.Contains(msg, "list idle") {
		idleMacros := p.macroManager.GetIdleBehaviorMacros()
		if len(idleMacros) == 0 {
			p.corradeClient.Say("No idle behavior macros configured.")
		} else {
			macroNames := make([]string, len(idleMacros))
			for i, macro := range idleMacros {
				macroNames[i] = macro.Name
			}
			p.corradeClient.Say(fmt.Sprintf("Idle behaviors: %s", strings.Join(macroNames, ", ")))
		}
		return true
	}

	return false
}

// followAvatar starts following a specific avatar
func (p *Processor) followAvatar(avatar string) error {
	// First get avatar position
	pos, err := p.corradeClient.GetAvatarPosition(avatar)
	if err != nil {
		return err
	}

	// Set follow target
	p.followTarget = &types.FollowTarget{
		Avatar:   avatar,
		LastSeen: time.Now(),
		Position: pos,
	}
	p.isFollowing = true
	p.corradeClient.SetFollowing(true, avatar)

	return nil
}

// stopFollowing stops following the current target
func (p *Processor) stopFollowing() {
	p.isFollowing = false
	p.followTarget = nil
	p.corradeClient.SetFollowing(false, "")
}

// followRoutine continuously follows the target avatar
func (p *Processor) followRoutine(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !p.isFollowing || p.followTarget == nil {
				continue
			}

			// Get current position of target
			pos, err := p.corradeClient.GetAvatarPosition(p.followTarget.Avatar)
			if err != nil {
				log.Printf("Error getting avatar position: %v", err)
				continue
			}

			// Calculate distance to target
			ownPos := p.corradeClient.GetOwnPosition()
			distance := corrade.CalculateDistance(ownPos, pos)

			// Follow if target moved more than 2 units away
			if distance > 2.0 {
				p.corradeClient.WalkTo(pos.X, pos.Y, pos.Z)
				p.followTarget.Position = pos
				p.followTarget.LastSeen = time.Now()
			}

			// Stop following if target hasn't been seen for 5 minutes
			if time.Since(p.followTarget.LastSeen) > 5*time.Minute {
				p.stopFollowing()
				p.corradeClient.Say("I lost track of who I was following.")
			}
		}
	}
}

// getFallbackResponse returns predefined responses when AI is disabled or fails
func (p *Processor) getFallbackResponse(context, message string) string {
	fallbacks := p.config.Prompts.FallbackResponses
	
	switch context {
	case "greeting":
		return fallbacks.Greeting
	case "help":
		return fallbacks.Help
	case "general":
		// Check for some basic keywords to provide more specific responses
		lowerMsg := strings.ToLower(message)
		if strings.Contains(lowerMsg, "what") || strings.Contains(lowerMsg, "who") || 
		   strings.Contains(lowerMsg, "how") || strings.Contains(lowerMsg, "why") {
			return fallbacks.General
		}
		return fallbacks.General
	default:
		return fallbacks.Unknown
	}
}

// IsLlamaEnabled returns whether Llama chat is currently enabled
func (p *Processor) IsLlamaEnabled() bool {
	return p.llamaEnabled
}

// SetLlamaEnabled enables or disables Llama chat at runtime
func (p *Processor) SetLlamaEnabled(enabled bool) {
	p.llamaEnabled = enabled
	
	status := "disabled"
	if enabled {
		status = "enabled"
	}
	
	p.addLog(types.LogEntry{
		Timestamp: time.Now(),
		Type:      "system",
		Avatar:    "System",
		Message:   fmt.Sprintf("Llama chat %s", status),
	})
}

// getLlamaResponse gets a response from the Llama API
func (p *Processor) getLlamaResponse(prompt, context string) (string, error) {
	// Use different prompts based on context
	var finalPrompt string
	switch context {
	case "greeting":
		finalPrompt = p.buildPrompt(p.config.Prompts.GreetingPrompt, prompt)
	case "help":
		finalPrompt = p.buildPrompt(p.config.Prompts.HelpPrompt, prompt)
	case "chat":
		fallthrough
	default:
		finalPrompt = p.buildPrompt(p.config.Prompts.ChatPrompt, prompt)
	}

	req := types.LlamaRequest{
		Model:  p.config.Llama.Model,
		Prompt: p.config.Prompts.SystemPrompt + "\n\n" + finalPrompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	resp, err := p.httpClient.Post(p.config.Llama.URL+"/api/generate", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var llamaResp types.LlamaResponse
	if err := json.Unmarshal(body, &llamaResp); err != nil {
		return "", err
	}

	return strings.TrimSpace(llamaResp.Response), nil
}

// buildPrompt builds a prompt with variable substitution
func (p *Processor) buildPrompt(template, userMessage string) string {
	prompt := strings.ReplaceAll(template, "{message}", userMessage)
	prompt = strings.ReplaceAll(prompt, "{botname}", p.config.Bot.Name)
	prompt = strings.ReplaceAll(prompt, "{maxlen}", fmt.Sprintf("%d", p.config.Bot.MaxMessageLen))
	return prompt
}

// addLog adds a log entry
func (p *Processor) addLog(entry types.LogEntry) {
	p.logsMutex.Lock()
	defer p.logsMutex.Unlock()

	p.logs = append(p.logs, entry)

	// Keep only last 1000 entries
	if len(p.logs) > 1000 {
		p.logs = p.logs[len(p.logs)-1000:]
	}
}

// GetLogs returns recent log entries
func (p *Processor) GetLogs(count int) []types.LogEntry {
	p.logsMutex.RLock()
	defer p.logsMutex.RUnlock()

	if count <= 0 || count > len(p.logs) {
		count = len(p.logs)
	}

	// Return most recent entries
	start := len(p.logs) - count
	if start < 0 {
		start = 0
	}

	return p.logs[start:]
}

// IsFollowing returns whether the bot is currently following someone
func (p *Processor) IsFollowing() bool {
	return p.isFollowing
}

// GetFollowTarget returns the current follow target
func (p *Processor) GetFollowTarget() *types.FollowTarget {
	return p.followTarget
}

// idleBehaviorRoutine runs idle behaviors when the bot is inactive
func (p *Processor) idleBehaviorRoutine(ctx context.Context) {
	idleTimeout := time.Duration(p.config.Bot.IdleTimeout) * time.Minute
	
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.idleBehaviorStopChan:
			return
		case <-ticker.C:
			// Check if any idle behavior macros are defined
			idleMacros := p.macroManager.GetIdleBehaviorMacros()
			if len(idleMacros) == 0 {
				// No idle behaviors defined, skip this cycle
				continue
			}

			// Check if we should start idle behaviors
			timeSinceLastInteraction := time.Since(p.lastInteractionTime)
			
			if timeSinceLastInteraction >= idleTimeout && !p.idleBehaviorRunning {
				// Start idle behavior routine
				p.idleBehaviorRunning = true
				go p.runIdleBehaviors(ctx)
			}
		}
	}
}

// runIdleBehaviors continuously runs random idle behaviors
func (p *Processor) runIdleBehaviors(ctx context.Context) {
	defer func() {
		p.idleBehaviorRunning = false
	}()

	log.Println("Starting idle behavior routine")
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.idleBehaviorStopChan:
			return
		default:
			// Check if idle behaviors are still available
			idleMacros := p.macroManager.GetIdleBehaviorMacros()
			if len(idleMacros) == 0 {
				log.Println("No idle behavior macros available, stopping idle routine")
				return
			}

			// Check if we should stop idle behaviors (new interaction)
			timeSinceLastInteraction := time.Since(p.lastInteractionTime)
			idleTimeout := time.Duration(p.config.Bot.IdleTimeout) * time.Minute
			
			if timeSinceLastInteraction < idleTimeout {
				log.Println("Stopping idle behavior routine due to interaction")
				return
			}

			// Don't run idle behaviors if following someone or recording
			if p.isFollowing {
				// Wait a shorter time and check again
				time.Sleep(1 * time.Minute)
				continue
			}

			if recording := p.macroManager.GetRecordingStatus(); recording != nil {
				time.Sleep(1 * time.Minute)
				continue
			}

			// Try to play a random idle behavior
			if err := p.macroManager.PlayRandomIdleBehavior(); err != nil {
				log.Printf("Could not play idle behavior: %v", err)
				// If we can't play idle behaviors, wait before trying again
				time.Sleep(5 * time.Minute)
				continue
			} else {
				p.addLog(types.LogEntry{
					Timestamp: time.Now(),
					Type:      "system",
					Avatar:    "System",
					Message:   "Performed idle behavior",
				})
			}

			// Calculate random wait time between min and max intervals
			nextInterval := p.getRandomIdleInterval()
			log.Printf("Next idle behavior in %.1f minutes", nextInterval.Minutes())

			// Wait for the random interval before next idle behavior
			select {
			case <-ctx.Done():
				return
			case <-p.idleBehaviorStopChan:
				return
			case <-time.After(nextInterval):
				continue
			}
		}
	}
}

// getRandomIdleInterval returns a random duration between min and max idle intervals
func (p *Processor) getRandomIdleInterval() time.Duration {
	minInterval := p.config.Bot.IdleBehaviorMinInterval
	maxInterval := p.config.Bot.IdleBehaviorMaxInterval
	
	// Validate configuration - ensure max >= min
	if maxInterval <= minInterval {
		log.Printf("Warning: maxInterval (%d) <= minInterval (%d), using minInterval", maxInterval, minInterval)
		return time.Duration(minInterval) * time.Minute
	}
	
	// Generate random minutes between min and max (inclusive)
	randomMinutes := rand.Intn(maxInterval-minInterval+1) + minInterval
	return time.Duration(randomMinutes) * time.Minute
}

// StopIdleBehaviors stops the idle behavior routine
func (p *Processor) StopIdleBehaviors() {
	if p.idleBehaviorRunning {
		close(p.idleBehaviorStopChan)
		p.idleBehaviorStopChan = make(chan struct{})
	}
}

// IsIdle returns whether the bot is currently in idle mode and has idle behaviors available
func (p *Processor) IsIdle() bool {
	// Check if any idle behavior macros are defined
	idleMacros := p.macroManager.GetIdleBehaviorMacros()
	if len(idleMacros) == 0 {
		return false // Not considered idle if no idle behaviors are defined
	}

	idleTimeout := time.Duration(p.config.Bot.IdleTimeout) * time.Minute
	return time.Since(p.lastInteractionTime) >= idleTimeout
}

// handleSitCommand processes sit commands
func (p *Processor) handleSitCommand(objectName, avatar string) error {
	// Try to sit on the object directly
	err := p.corradeClient.SitOn(objectName)
	if err != nil {
		p.corradeClient.Say("I couldn't find that object to sit on.")
		log.Printf("Sit error: %v", err)
		return err
	}
	
	p.corradeClient.Say(fmt.Sprintf("Sitting on %s", objectName))
	p.recordAction("sit", map[string]interface{}{
		"object": objectName,
	})
	return nil
}

// handleSitConfirmation processes sit confirmation responses (currently disabled)
// This was removed because FindNearbyObjects method doesn't exist in corrade.Client
func (p *Processor) handleSitConfirmation(message types.ChatMessage) bool {
	// This functionality has been simplified - no longer doing partial matching
	return false
}

// sitConfirmationTimeout handles timeout for sit confirmations (currently disabled)
func (p *Processor) sitConfirmationTimeout() {
	// This functionality has been simplified - no longer needed
}

// parseChoice parses a numeric choice from user input (currently disabled)
func parseChoice(input string) (int, error) {
	// This functionality has been simplified - no longer needed
	return 0, fmt.Errorf("choice parsing disabled")
}

// recordAction records an action if currently recording a macro
func (p *Processor) recordAction(actionType string, data map[string]interface{}) {
	if p.macroManager != nil {
		p.macroManager.RecordAction(actionType, data)
	}
}

// GetMacroManager returns the macro manager for external access
func (p *Processor) GetMacroManager() *macros.Manager {
	return p.macroManager
}

// GetPendingSitRequest returns the current pending sit confirmation request (simplified)
func (p *Processor) GetPendingSitRequest() *types.PendingSitConfirmation {
	// This functionality has been simplified since FindNearbyObjects doesn't exist
	// Always return nil for now
	return nil
}

// Add this method to expose HandleNotification for the web interface
func (p *Processor) ProcessNotification(notification map[string]interface{}) {
	p.HandleNotification(notification)
}
