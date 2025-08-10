package macros

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"slbot/internal/config"
	"slbot/internal/corrade"
	"slbot/internal/types"
)

const (
	MacrosDir = "macros"
	MacroExt  = ".json"
)

// Manager handles macro recording and playback
type Manager struct {
	config        *config.Config
	corradeClient *corrade.Client
	macros        map[string]*types.Macro
	recording     *types.MacroRecording
	isPlaying     bool
	mutex         sync.RWMutex
}

// NewManager creates a new macro manager
func NewManager(cfg *config.Config, corradeClient *corrade.Client) *Manager {
	manager := &Manager{
		config:        cfg,
		corradeClient: corradeClient,
		macros:        make(map[string]*types.Macro),
		recording:     nil,
		isPlaying:     false,
	}

	// Create macros directory if it doesn't exist
	if err := os.MkdirAll(MacrosDir, 0755); err != nil {
		log.Printf("Failed to create macros directory: %v", err)
	}

	// Load existing macros
	if err := manager.loadMacros(); err != nil {
		log.Printf("Failed to load macros: %v", err)
	}

	return manager
}

// IsOwner checks if the given username is an owner
func (m *Manager) IsOwner(username string) bool {
	for _, owner := range m.config.Bot.Owners {
		if strings.EqualFold(owner, username) {
			return true
		}
	}
	return false
}

// StartRecording begins recording a new macro
func (m *Manager) StartRecording(name, recordedBy string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.IsOwner(recordedBy) {
		return fmt.Errorf("access denied: %s is not an owner", recordedBy)
	}

	if m.recording != nil && m.recording.IsRecording {
		return fmt.Errorf("already recording macro: %s", m.recording.Name)
	}

	if m.isPlaying {
		return fmt.Errorf("cannot record while playing a macro")
	}

	// Validate macro name
	if name == "" {
		return fmt.Errorf("macro name cannot be empty")
	}

	if strings.ContainsAny(name, "/\\:*?\"<>|") {
		return fmt.Errorf("macro name contains invalid characters")
	}

	m.recording = &types.MacroRecording{
		Name:        name,
		StartTime:   time.Now(),
		Actions:     make([]types.MacroAction, 0),
		RecordedBy:  recordedBy,
		IsRecording: true,
	}

	log.Printf("Started recording macro '%s' by %s", name, recordedBy)
	return nil
}

// StopRecording stops the current recording and saves the macro
func (m *Manager) StopRecording(description string, tags []string, isIdleBehavior, isAutoGreet bool) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.recording == nil || !m.recording.IsRecording {
		return fmt.Errorf("no active recording")
	}

	if len(m.recording.Actions) == 0 {
		return fmt.Errorf("cannot save empty macro")
	}

	// Create macro from recording
	duration := time.Since(m.recording.StartTime)
	macro := &types.Macro{
		Name:         m.recording.Name,
		Description:  description,
		Actions:      m.recording.Actions,
		CreatedBy:    m.recording.RecordedBy,
		CreatedAt:    m.recording.StartTime,
		Duration:     duration,
		Tags:         tags,
		IdleBehavior: isIdleBehavior,
		AutoGreet:    isAutoGreet,
	}

	// Save macro to file
	if err := m.saveMacro(macro); err != nil {
		return fmt.Errorf("failed to save macro: %w", err)
	}

	// Add to memory
	m.macros[macro.Name] = macro

	log.Printf("Saved macro '%s' with %d actions (duration: %v, idle: %v, autogreet: %v)",
		macro.Name, len(macro.Actions), duration, isIdleBehavior, isAutoGreet)

	// Clear recording
	m.recording = nil

	return nil
}

// CancelRecording cancels the current recording without saving
func (m *Manager) CancelRecording() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.recording == nil || !m.recording.IsRecording {
		return fmt.Errorf("no active recording")
	}

	name := m.recording.Name
	m.recording = nil

	log.Printf("Cancelled recording of macro '%s'", name)
	return nil
}

// RecordAction adds an action to the current recording
func (m *Manager) RecordAction(actionType string, data map[string]interface{}) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.recording == nil || !m.recording.IsRecording {
		return
	}

	action := types.MacroAction{
		Type:      actionType,
		Timestamp: time.Now(),
		Data:      data,
	}

	m.recording.Actions = append(m.recording.Actions, action)
	log.Printf("Recorded action: %s", actionType)
}

// PlayMacro executes a saved macro
func (m *Manager) PlayMacro(name, requestedBy string) error {
	m.mutex.Lock()

	if !m.IsOwner(requestedBy) && requestedBy != "AutoGreet" {
		m.mutex.Unlock()
		return fmt.Errorf("access denied: %s is not an owner", requestedBy)
	}

	if m.isPlaying {
		m.mutex.Unlock()
		return fmt.Errorf("already playing a macro")
	}

	if m.recording != nil && m.recording.IsRecording {
		m.mutex.Unlock()
		return fmt.Errorf("cannot play macro while recording")
	}

	macro, exists := m.macros[name]
	if !exists {
		m.mutex.Unlock()
		return fmt.Errorf("macro '%s' not found", name)
	}

	m.isPlaying = true
	m.mutex.Unlock()

	// Execute macro in goroutine
	go func() {
		defer func() {
			m.mutex.Lock()
			m.isPlaying = false
			m.mutex.Unlock()
		}()

		log.Printf("Playing macro '%s' (%d actions)", name, len(macro.Actions))

		startTime := time.Now()
		for i, action := range macro.Actions {
			// Calculate delay based on original timing
			if i > 0 {
				prevAction := macro.Actions[i-1]
				delay := action.Timestamp.Sub(prevAction.Timestamp)
				if delay > 0 && delay < 30*time.Second { // Cap max delay
					time.Sleep(delay)
				}
			}

			if err := m.executeAction(action); err != nil {
				log.Printf("Error executing action %d in macro '%s': %v", i+1, name, err)
			}
		}

		log.Printf("Completed macro '%s' in %v", name, time.Since(startTime))
	}()

	return nil
}

// executeAction performs a single macro action
func (m *Manager) executeAction(action types.MacroAction) error {
	switch action.Type {
	case "walk":
		if x, ok := action.Data["x"].(float64); ok {
			if y, ok := action.Data["y"].(float64); ok {
				if z, ok := action.Data["z"].(float64); ok {
					return m.corradeClient.WalkTo(x, y, z)
				}
			}
		}
		return fmt.Errorf("invalid walk action data")

	case "teleport":
		if region, ok := action.Data["region"].(string); ok {
			if x, ok := action.Data["x"].(float64); ok {
				if y, ok := action.Data["y"].(float64); ok {
					if z, ok := action.Data["z"].(float64); ok {
						return m.corradeClient.Teleport(region, x, y, z)
					}
				}
			}
		}
		return fmt.Errorf("invalid teleport action data")

	case "sit":
		if object, ok := action.Data["object"].(string); ok {
			return m.corradeClient.SitOn(object)
		}
		return fmt.Errorf("invalid sit action data")

	case "stand":
		return m.corradeClient.StandUp()

	case "tell":
		if message, ok := action.Data["message"].(string); ok {
			return m.corradeClient.Tell(message)
		}
		return fmt.Errorf("invalid tell action data")

	case "whisper":
		if avatar, ok := action.Data["avatar"].(string); ok {
			if message, ok := action.Data["message"].(string); ok {
				return m.corradeClient.Whisper(avatar, message)
			}
		}
		return fmt.Errorf("invalid whisper action data")

	case "wait":
		if duration, ok := action.Data["duration"].(float64); ok {
			time.Sleep(time.Duration(duration) * time.Millisecond)
			return nil
		}
		return fmt.Errorf("invalid wait action data")

	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

// GetMacros returns all available macros
func (m *Manager) GetMacros() map[string]*types.Macro {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]*types.Macro)
	for name, macro := range m.macros {
		result[name] = macro
	}
	return result
}

// GetIdleBehaviorMacros returns all macros tagged as idle behaviors
func (m *Manager) GetIdleBehaviorMacros() []*types.Macro {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var idleMacros []*types.Macro
	for _, macro := range m.macros {
		if macro.IdleBehavior {
			idleMacros = append(idleMacros, macro)
		}
	}
	return idleMacros
}

// GetAutoGreetMacros returns all macros tagged as auto-greet macros
func (m *Manager) GetAutoGreetMacros() []*types.Macro {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var autoGreetMacros []*types.Macro
	for _, macro := range m.macros {
		if macro.AutoGreet {
			autoGreetMacros = append(autoGreetMacros, macro)
		}
	}
	return autoGreetMacros
}

// SetIdleBehavior marks a macro as idle behavior or removes the marking
func (m *Manager) SetIdleBehavior(name, requestedBy string, isIdleBehavior bool) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.IsOwner(requestedBy) {
		return fmt.Errorf("access denied: %s is not an owner", requestedBy)
	}

	macro, exists := m.macros[name]
	if !exists {
		return fmt.Errorf("macro '%s' not found", name)
	}

	macro.IdleBehavior = isIdleBehavior

	// Save updated macro
	if err := m.saveMacro(macro); err != nil {
		return fmt.Errorf("failed to update macro: %w", err)
	}

	status := "removed from"
	if isIdleBehavior {
		status = "added to"
	}
	log.Printf("Macro '%s' %s idle behaviors by %s", name, status, requestedBy)

	return nil
}

// SetAutoGreet marks a macro as auto-greet or removes the marking
func (m *Manager) SetAutoGreet(name, requestedBy string, isAutoGreet bool) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.IsOwner(requestedBy) {
		return fmt.Errorf("access denied: %s is not an owner", requestedBy)
	}

	macro, exists := m.macros[name]
	if !exists {
		return fmt.Errorf("macro '%s' not found", name)
	}

	macro.AutoGreet = isAutoGreet

	// Save updated macro
	if err := m.saveMacro(macro); err != nil {
		return fmt.Errorf("failed to update macro: %w", err)
	}

	status := "removed from"
	if isAutoGreet {
		status = "added to"
	}
	log.Printf("Macro '%s' %s auto-greet macros by %s", name, status, requestedBy)

	return nil
}

// PlayRandomIdleBehavior plays a random idle behavior macro
func (m *Manager) PlayRandomIdleBehavior() error {
	idleMacros := m.GetIdleBehaviorMacros()
	if len(idleMacros) == 0 {
		return fmt.Errorf("no idle behavior macros available")
	}

	m.mutex.Lock()
	if m.isPlaying {
		m.mutex.Unlock()
		return fmt.Errorf("already playing a macro")
	}

	if m.recording != nil && m.recording.IsRecording {
		m.mutex.Unlock()
		return fmt.Errorf("cannot play macro while recording")
	}

	// Select random idle macro
	selectedMacro := idleMacros[time.Now().UnixNano()%int64(len(idleMacros))]

	m.isPlaying = true
	m.mutex.Unlock()

	// Execute macro in goroutine
	go func() {
		defer func() {
			m.mutex.Lock()
			m.isPlaying = false
			m.mutex.Unlock()
		}()

		log.Printf("Playing idle behavior macro '%s' (%d actions)", selectedMacro.Name, len(selectedMacro.Actions))

		startTime := time.Now()
		for i, action := range selectedMacro.Actions {
			// Calculate delay based on original timing
			if i > 0 {
				prevAction := selectedMacro.Actions[i-1]
				delay := action.Timestamp.Sub(prevAction.Timestamp)
				if delay > 0 && delay < 30*time.Second { // Cap max delay
					time.Sleep(delay)
				}
			}

			if err := m.executeAction(action); err != nil {
				log.Printf("Error executing action %d in idle macro '%s': %v", i+1, selectedMacro.Name, err)
			}
		}

		log.Printf("Completed idle behavior macro '%s' in %v", selectedMacro.Name, time.Since(startTime))
	}()

	return nil
}

// PlayAutoGreetMacro plays the specified auto-greet macro for a new avatar
func (m *Manager) PlayAutoGreetMacro(macroName, avatarName string) error {
	m.mutex.Lock()

	if m.isPlaying {
		m.mutex.Unlock()
		return fmt.Errorf("already playing a macro")
	}

	if m.recording != nil && m.recording.IsRecording {
		m.mutex.Unlock()
		return fmt.Errorf("cannot play macro while recording")
	}

	macro, exists := m.macros[macroName]
	if !exists {
		m.mutex.Unlock()
		return fmt.Errorf("auto-greet macro '%s' not found", macroName)
	}

	m.isPlaying = true
	m.mutex.Unlock()

	// Execute macro in goroutine
	go func() {
		defer func() {
			m.mutex.Lock()
			m.isPlaying = false
			m.mutex.Unlock()
		}()

		log.Printf("Playing auto-greet macro '%s' for %s (%d actions)", macroName, avatarName, len(macro.Actions))

		startTime := time.Now()
		for i, action := range macro.Actions {
			// Calculate delay based on original timing
			if i > 0 {
				prevAction := macro.Actions[i-1]
				delay := action.Timestamp.Sub(prevAction.Timestamp)
				if delay > 0 && delay < 30*time.Second { // Cap max delay
					time.Sleep(delay)
				}
			}

			// For auto-greet macros, we can substitute {avatar} in messages
			if action.Type == "tell" || action.Type == "whisper" {
				if message, ok := action.Data["message"].(string); ok {
					// Replace {avatar} placeholder with the actual avatar name
					message = strings.ReplaceAll(message, "{avatar}", avatarName)
					action.Data["message"] = message
				}
			}

			if err := m.executeAction(action); err != nil {
				log.Printf("Error executing action %d in auto-greet macro '%s': %v", i+1, macroName, err)
			}
		}

		log.Printf("Completed auto-greet macro '%s' for %s in %v", macroName, avatarName, time.Since(startTime))
	}()

	return nil
}

// GetMacro returns a specific macro
func (m *Manager) GetMacro(name string) (*types.Macro, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	macro, exists := m.macros[name]
	return macro, exists
}

// DeleteMacro removes a macro
func (m *Manager) DeleteMacro(name, requestedBy string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.IsOwner(requestedBy) {
		return fmt.Errorf("access denied: %s is not an owner", requestedBy)
	}

	if _, exists := m.macros[name]; !exists {
		return fmt.Errorf("macro '%s' not found", name)
	}

	// Remove from file system
	filename := filepath.Join(MacrosDir, name+MacroExt)
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete macro file: %w", err)
	}

	// Remove from memory
	delete(m.macros, name)

	log.Printf("Deleted macro '%s' by %s", name, requestedBy)
	return nil
}

// GetRecordingStatus returns current recording status
func (m *Manager) GetRecordingStatus() *types.MacroRecording {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.recording == nil {
		return nil
	}

	// Return a copy
	recording := *m.recording
	return &recording
}

// IsPlaying returns whether a macro is currently playing
func (m *Manager) IsPlaying() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.isPlaying
}

// saveMacro saves a macro to disk
func (m *Manager) saveMacro(macro *types.Macro) error {
	filename := filepath.Join(MacrosDir, macro.Name+MacroExt)

	data, err := json.MarshalIndent(macro, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// loadMacros loads all macros from disk
func (m *Manager) loadMacros() error {
	files, err := filepath.Glob(filepath.Join(MacrosDir, "*"+MacroExt))
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := m.loadMacro(file); err != nil {
			log.Printf("Failed to load macro from %s: %v", file, err)
		}
	}

	log.Printf("Loaded %d macros", len(m.macros))
	return nil
}

// loadMacro loads a single macro from disk
func (m *Manager) loadMacro(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var macro types.Macro
	if err := json.Unmarshal(data, &macro); err != nil {
		return err
	}

	m.macros[macro.Name] = &macro
	return nil
}
