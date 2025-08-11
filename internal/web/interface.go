package web

import (
	"context"
   "strings"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"slbot/internal/chat"
	"slbot/internal/config"
	"slbot/internal/corrade"
	"slbot/internal/types"
)

// BuildInfo holds build-time information
type BuildInfo struct {
	Version   string
	BuildTime string
	BuildUser string
	BuildHost string
	GitCommit string
	GoVersion string
	GoModules map[string]string
}

// SystemInfo holds runtime system information
type SystemInfo struct {
	GoVersion    string
	NumCPU       int
	NumGoroutine int
	MemStats     runtime.MemStats
	Uptime       time.Duration
	OS           string
	Arch         string
}

// Interface handles the web dashboard
type Interface struct {
	config        *config.Config
	corradeClient *corrade.Client
	chatProcessor *chat.Processor
	server        *http.Server
	templates     *template.Template
	buildInfo     BuildInfo
	startTime     time.Time
	callbackURL   string // ADD THIS LINE
}

// Updated NewInterface function
func NewInterface(cfg *config.Config, corradeClient *corrade.Client, chatProcessor *chat.Processor) *Interface {
	// Construct callback URL based on web port
	callbackURL := fmt.Sprintf("http://localhost:%d/corrade/notifications", cfg.Bot.WebPort)
	
	return &Interface{
		config:        cfg,
		corradeClient: corradeClient,
		chatProcessor: chatProcessor,
		startTime:     time.Now(),
		callbackURL:   callbackURL, // ADD THIS LINE
		buildInfo: BuildInfo{
			Version:   getVersion(),
			BuildTime: getBuildTime(),
			BuildUser: getBuildUser(),
			BuildHost: getBuildHost(),
			GitCommit: getGitCommit(),
			GoVersion: runtime.Version(),
			GoModules: getGoModules(),
		},
	}
}

// Build-time variables (set via ldflags)
var (
	Version   = "dev"
	BuildTime = "unknown"
	BuildUser = "unknown"
	BuildHost = "unknown"
	GitCommit = "unknown"
)

func getVersion() string   { return Version }
func getBuildTime() string { return BuildTime }
func getBuildUser() string { return BuildUser }
func getBuildHost() string { return BuildHost }
func getGitCommit() string { return GitCommit }

// getGoModules returns information about Go modules (simplified version)
func getGoModules() map[string]string {
	// In a real implementation, you might parse go.mod or use build info
	return map[string]string{
		"github.com/gorilla/mux": "v1.8.0",
		// Add other modules as needed
	}
}

// Start starts the web interface server
func (w *Interface) Start(ctx context.Context) error {
	// Load templates
	if err := w.loadTemplates(); err != nil {
		return fmt.Errorf("failed to load templates: %w", err)
	}

	// Setup routes
	router := mux.NewRouter()

	// Static files
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("web/static/"))))

	// Dashboard
	router.HandleFunc("/", w.dashboardHandler).Methods("GET")

	// Corrade notification endpoint
	router.HandleFunc("/corrade/notifications", w.corradeNotificationHandler).Methods("POST")

	// API endpoints
	api := router.PathPrefix("/api").Subrouter()
	api.HandleFunc("/status", w.statusHandler).Methods("GET")
	api.HandleFunc("/system", w.systemInfoHandler).Methods("GET")
	api.HandleFunc("/build", w.buildInfoHandler).Methods("GET")
	api.HandleFunc("/logs", w.logsHandler).Methods("GET")
	api.HandleFunc("/teleport", w.teleportHandler).Methods("POST")
	api.HandleFunc("/walk", w.walkHandler).Methods("POST")
	api.HandleFunc("/stop-following", w.stopFollowingHandler).Methods("POST")
	api.HandleFunc("/stand", w.standHandler).Methods("POST")
	api.HandleFunc("/toggle-llama", w.toggleLlamaHandler).Methods("POST")

	// Avatar tracking API endpoints
	api.HandleFunc("/avatars", w.getAvatarsHandler).Methods("GET")
	api.HandleFunc("/autogreet", w.getAutoGreetHandler).Methods("GET")
	api.HandleFunc("/autogreet", w.setAutoGreetHandler).Methods("POST")
	api.HandleFunc("/autogreet", w.disableAutoGreetHandler).Methods("DELETE")

	// Macro API endpoints
	macroAPI := api.PathPrefix("/macros").Subrouter()
	macroAPI.HandleFunc("", w.getMacrosHandler).Methods("GET")
	macroAPI.HandleFunc("/play/{name}", w.playMacroHandler).Methods("POST")
	macroAPI.HandleFunc("/delete/{name}", w.deleteMacroHandler).Methods("DELETE")
	macroAPI.HandleFunc("/recording", w.getRecordingStatusHandler).Methods("GET")
	macroAPI.HandleFunc("/idle/{name}", w.setIdleBehaviorHandler).Methods("POST")
	macroAPI.HandleFunc("/idle/{name}", w.unsetIdleBehaviorHandler).Methods("DELETE")
	macroAPI.HandleFunc("/autogreet/{name}", w.setAutoGreetMacroHandler).Methods("POST")
	macroAPI.HandleFunc("/autogreet/{name}", w.unsetAutoGreetMacroHandler).Methods("DELETE")

	// Create server
	w.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", w.config.Bot.WebPort),
		Handler: router,
	}

	log.Printf("Web interface starting on http://localhost:%d", w.config.Bot.WebPort)

	// Start periodic status updates
	go w.statusUpdateRoutine(ctx)

	// Start periodic avatar tracking (NEW)
	go w.avatarTrackingRoutine(ctx)

	// Start server
	if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

// Stop stops the web interface server
func (w *Interface) Stop(ctx context.Context) error {
	if w.server != nil {
		return w.server.Shutdown(ctx)
	}
	return nil
}

// avatarTrackingRoutine periodically requests nearby avatars (NEW)
func (w *Interface) avatarTrackingRoutine(ctx context.Context) {
	// Initial delay to let everything start up
	time.Sleep(5 * time.Second)
	
	// Request avatar tracking every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Do an immediate request
	if err := w.corradeClient.RequestNearbyAvatars(w.callbackURL); err != nil {
		log.Printf("Initial avatar tracking request failed: %v", err)
	} else {
		log.Printf("Started avatar tracking with callback URL: %s", w.callbackURL)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.corradeClient.RequestNearbyAvatars(w.callbackURL); err != nil {
				log.Printf("Avatar tracking request failed: %v", err)
			}
		}
	}
}

// corradeNotificationHandler handles notifications from Corrade (UPDATED)
func (w *Interface) corradeNotificationHandler(writer http.ResponseWriter, request *http.Request) {
	var notification map[string]interface{}

	// Check content type to handle both JSON and form-encoded data
	contentType := request.Header.Get("Content-Type")
	
	if strings.Contains(contentType, "application/json") {
		// Handle JSON data
		if err := json.NewDecoder(request.Body).Decode(&notification); err != nil {
			log.Printf("Error decoding Corrade JSON notification: %v", err)
			http.Error(writer, "Bad Request", http.StatusBadRequest)
			return
		}
	} else if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// Handle form-encoded data
		if err := request.ParseForm(); err != nil {
			log.Printf("Error parsing Corrade form notification: %v", err)
			http.Error(writer, "Bad Request", http.StatusBadRequest)
			return
		}
		
		// Convert form values to map[string]interface{}
		notification = make(map[string]interface{})
		for key, values := range request.Form {
			if len(values) == 1 {
				// Single value - try to parse as JSON first, fallback to string
				value := values[0]
				var jsonValue interface{}
				if err := json.Unmarshal([]byte(value), &jsonValue); err == nil {
					notification[key] = jsonValue
				} else {
					notification[key] = value
				}
			} else {
				// Multiple values - keep as string slice
				notification[key] = values
			}
		}
	} else {
		log.Printf("Unsupported content type: %s", contentType)
		http.Error(writer, "Unsupported Media Type", http.StatusUnsupportedMediaType)
		return
	}

	// Route callbacks based on command type (NEW LOGIC)
	if command, ok := notification["command"].(string); ok {
		switch command {
		case "getmapavatarpositions":
			log.Printf("Received getmapavatarpositions callback")
			w.corradeClient.ProcessMapAvatarPositionsCallback(notification)
		case "getavatardata":
			log.Printf("Received getavatardata callback")
			w.corradeClient.ProcessAvatarDataCallback(notification)
		default:
			// Handle other notifications through chat processor (chat, IM, etc.)
			w.chatProcessor.ProcessNotification(notification)
		}
	} else {
		// If no command specified, try to determine from other fields
		if _, hasData := notification["data"]; hasData {
			// Likely an avatar positions callback
			log.Printf("Received callback with data field (likely avatar positions)")
			w.corradeClient.ProcessMapAvatarPositionsCallback(notification)
		} else {
			// Default to chat processor
			w.chatProcessor.ProcessNotification(notification)
		}
	}

	// Extract avatar names from chat notifications for name mapping (NEW)
	if msgType, ok := notification["type"].(string); ok && (msgType == "chat" || msgType == "instantmessage") {
		if firstName, hasFirst := notification["firstname"].(string); hasFirst {
			if uuid, hasUUID := notification["agent"].(string); hasUUID {
				lastName := ""
				if ln, hasLast := notification["lastname"].(string); hasLast && ln != "Resident" {
					lastName = ln
				}
				
				fullName := firstName
				if lastName != "" {
					fullName += " " + lastName
				}
				
				// Update the name mapping in Corrade client
				w.corradeClient.UpdateAvatarName(uuid, fullName)
				log.Printf("Updated name mapping: %s -> %s", uuid, fullName)
			}
		}
	}

	// Respond with success
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("OK"))
}

// refreshAvatarsHandler manually triggers avatar refresh (NEW)
func (w *Interface) refreshAvatarsHandler(writer http.ResponseWriter, request *http.Request) {
	err := w.corradeClient.RequestNearbyAvatars(w.callbackURL)

	response := map[string]string{
		"status":  "success",
		"message": "Avatar refresh requested",
	}

	if err != nil {
		response["status"] = "error"
		response["message"] = "Failed to refresh avatars: " + err.Error()
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// loadTemplates loads HTML templates
func (w *Interface) loadTemplates() error {
	templatePath := filepath.Join("web", "templates", "*.html")
	templates, err := template.New("").Funcs(template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
		"formatDuration": func(d time.Duration) string {
			if d < time.Minute {
				return fmt.Sprintf("%.0fs", d.Seconds())
			}
			return fmt.Sprintf("%.0fm", d.Minutes())
		},
		"formatBytes": func(bytes uint64) string {
			const unit = 1024
			if bytes < unit {
				return fmt.Sprintf("%d B", bytes)
			}
			div, exp := int64(unit), 0
			for n := bytes / unit; n >= unit; n /= unit {
				div *= unit
				exp++
			}
			return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
		},
		"formatUptime": func(d time.Duration) string {
			days := int(d.Hours()) / 24
			hours := int(d.Hours()) % 24
			minutes := int(d.Minutes()) % 60
			if days > 0 {
				return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
			}
			if hours > 0 {
				return fmt.Sprintf("%dh %dm", hours, minutes)
			}
			return fmt.Sprintf("%dm", minutes)
		},
	}).ParseGlob(templatePath)
	if err != nil {
		return err
	}
	w.templates = templates
	return nil
}

// statusUpdateRoutine periodically updates bot status
func (w *Interface) statusUpdateRoutine(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.corradeClient.UpdateStatusWithConfig(w.config)
		}
	}
}

// HTTP Handlers

// dashboardHandler serves the main dashboard
func (w *Interface) dashboardHandler(writer http.ResponseWriter, request *http.Request) {
	status := w.corradeClient.GetStatus()
	logs := w.chatProcessor.GetLogs(50)
	macros := w.chatProcessor.GetMacroManager().GetMacros()
	recordingStatus := w.chatProcessor.GetMacroManager().GetRecordingStatus()
	isIdle := w.chatProcessor.IsIdle()
	nearbyAvatars := w.chatProcessor.GetNearbyAvatars()
	autoGreetEnabled, autoGreetMacro := w.chatProcessor.GetAutoGreetConfig()
	systemInfo := w.getSystemInfo()

	data := struct {
		Status           types.BotStatus
		Logs             []types.LogEntry
		LlamaEnabled     bool
		Macros           map[string]*types.Macro
		IsRecording      bool
		RecordingStatus  *types.MacroRecording
		IsIdle           bool
		NearbyAvatars    map[string]*types.AvatarInfo
		AutoGreetEnabled bool
		AutoGreetMacro   string
		BuildInfo        BuildInfo
		SystemInfo       SystemInfo
	}{
		Status:           status,
		Logs:             logs,
		LlamaEnabled:     w.chatProcessor.IsLlamaEnabled(),
		Macros:           macros,
		IsRecording:      recordingStatus != nil,
		RecordingStatus:  recordingStatus,
		IsIdle:           isIdle,
		NearbyAvatars:    nearbyAvatars,
		AutoGreetEnabled: autoGreetEnabled,
		AutoGreetMacro:   autoGreetMacro,
		BuildInfo:        w.buildInfo,
		SystemInfo:       systemInfo,
	}

	if err := w.templates.ExecuteTemplate(writer, "dashboard.html", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
	}
}

// getSystemInfo returns current system information
func (w *Interface) getSystemInfo() SystemInfo {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return SystemInfo{
		GoVersion:    runtime.Version(),
		NumCPU:       runtime.NumCPU(),
		NumGoroutine: runtime.NumGoroutine(),
		MemStats:     memStats,
		Uptime:       time.Since(w.startTime),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
	}
}

// systemInfoHandler returns system information as JSON
func (w *Interface) systemInfoHandler(writer http.ResponseWriter, request *http.Request) {
	systemInfo := w.getSystemInfo()

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(systemInfo)
}

// buildInfoHandler returns build information as JSON
func (w *Interface) buildInfoHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(w.buildInfo)
}

// statusHandler returns current bot status as JSON
func (w *Interface) statusHandler(writer http.ResponseWriter, request *http.Request) {
	status := w.corradeClient.UpdateStatusWithConfig(w.config)

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(status)
}

// logsHandler returns recent logs as JSON
func (w *Interface) logsHandler(writer http.ResponseWriter, request *http.Request) {
	countStr := request.URL.Query().Get("count")
	count := 50

	if countStr != "" {
		if c, err := strconv.Atoi(countStr); err == nil && c > 0 {
			count = c
		}
	}

	logs := w.chatProcessor.GetLogs(count)

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(logs)
}

// getAvatarsHandler returns nearby avatars as JSON
func (w *Interface) getAvatarsHandler(writer http.ResponseWriter, request *http.Request) {
	avatars := w.chatProcessor.GetNearbyAvatars()

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(avatars)
}

// getAutoGreetHandler returns current auto-greet configuration
func (w *Interface) getAutoGreetHandler(writer http.ResponseWriter, request *http.Request) {
	enabled, macroName := w.chatProcessor.GetAutoGreetConfig()

	response := map[string]interface{}{
		"enabled":   enabled,
		"macroName": macroName,
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// setAutoGreetHandler sets auto-greet configuration
func (w *Interface) setAutoGreetHandler(writer http.ResponseWriter, request *http.Request) {
	var req types.AutoGreetRequest
	if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
		http.Error(writer, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Enabled && req.MacroName == "" {
		http.Error(writer, "Macro name required when enabling auto-greet", http.StatusBadRequest)
		return
	}

	// Check if macro exists when enabling
	if req.Enabled {
		if _, exists := w.chatProcessor.GetMacroManager().GetMacro(req.MacroName); !exists {
			response := map[string]string{
				"status":  "error",
				"message": fmt.Sprintf("Macro '%s' not found", req.MacroName),
			}
			writer.Header().Set("Content-Type", "application/json")
			json.NewEncoder(writer).Encode(response)
			return
		}
	}

	w.chatProcessor.SetAutoGreetConfig(req.Enabled, req.MacroName)

	response := map[string]interface{}{
		"status":    "success",
		"message":   "Auto-greet configuration updated",
		"enabled":   req.Enabled,
		"macroName": req.MacroName,
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// disableAutoGreetHandler disables auto-greet
func (w *Interface) disableAutoGreetHandler(writer http.ResponseWriter, request *http.Request) {
	w.chatProcessor.SetAutoGreetConfig(false, "")

	response := map[string]string{
		"status":  "success",
		"message": "Auto-greet disabled",
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// teleportHandler handles teleport requests
func (w *Interface) teleportHandler(writer http.ResponseWriter, request *http.Request) {
	var req types.TeleportRequest
	if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
		http.Error(writer, "Invalid JSON", http.StatusBadRequest)
		return
	}

	err := w.corradeClient.Teleport(req.Region, req.X, req.Y, req.Z)

	response := map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Teleporting to %s (%.0f, %.0f, %.0f)", req.Region, req.X, req.Y, req.Z),
	}

	if err != nil {
		response["status"] = "error"
		response["message"] = "Failed to teleport: " + err.Error()
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// Macro API handlers

// getMacrosHandler returns all available macros
func (w *Interface) getMacrosHandler(writer http.ResponseWriter, request *http.Request) {
	macros := w.chatProcessor.GetMacroManager().GetMacros()

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(macros)
}

// playMacroHandler plays a specific macro
func (w *Interface) playMacroHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	macroName := vars["name"]

	if macroName == "" {
		http.Error(writer, "Macro name required", http.StatusBadRequest)
		return
	}

	// For web interface, use first owner as requestor
	requestor := "WebInterface"
	if len(w.config.Bot.Owners) > 0 {
		requestor = w.config.Bot.Owners[0]
	}

	err := w.chatProcessor.GetMacroManager().PlayMacro(macroName, requestor)

	response := map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Playing macro '%s'", macroName),
	}

	if err != nil {
		response["status"] = "error"
		response["message"] = err.Error()
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// deleteMacroHandler deletes a specific macro
func (w *Interface) deleteMacroHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	macroName := vars["name"]

	if macroName == "" {
		http.Error(writer, "Macro name required", http.StatusBadRequest)
		return
	}

	// For web interface, use first owner as requestor
	requestor := "WebInterface"
	if len(w.config.Bot.Owners) > 0 {
		requestor = w.config.Bot.Owners[0]
	}

	err := w.chatProcessor.GetMacroManager().DeleteMacro(macroName, requestor)

	response := map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Deleted macro '%s'", macroName),
	}

	if err != nil {
		response["status"] = "error"
		response["message"] = err.Error()
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// getRecordingStatusHandler returns current recording status
func (w *Interface) getRecordingStatusHandler(writer http.ResponseWriter, request *http.Request) {
	status := w.chatProcessor.GetMacroManager().GetRecordingStatus()

	response := map[string]interface{}{
		"isRecording": status != nil,
		"status":      status,
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// toggleLlamaHandler toggles Llama chat on/off
func (w *Interface) toggleLlamaHandler(writer http.ResponseWriter, request *http.Request) {
	currentStatus := w.chatProcessor.IsLlamaEnabled()
	w.chatProcessor.SetLlamaEnabled(!currentStatus)

	newStatus := "enabled"
	if currentStatus {
		newStatus = "disabled"
	}

	response := map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Llama chat %s", newStatus),
		"enabled": !currentStatus,
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// walkHandler handles walk requests
func (w *Interface) walkHandler(writer http.ResponseWriter, request *http.Request) {
	var req types.WalkRequest
	if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
		http.Error(writer, "Invalid JSON", http.StatusBadRequest)
		return
	}

	err := w.corradeClient.WalkTo(req.X, req.Y, req.Z)

	response := map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Walking to (%.0f, %.0f, %.0f)", req.X, req.Y, req.Z),
	}

	if err != nil {
		response["status"] = "error"
		response["message"] = "Failed to walk: " + err.Error()
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// stopFollowingHandler handles stop following requests
func (w *Interface) stopFollowingHandler(writer http.ResponseWriter, request *http.Request) {
	// This would need to be coordinated with the chat processor
	// For now, we'll just return success
	response := map[string]string{
		"status":  "success",
		"message": "Stop following command sent",
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// standHandler handles stand up requests
func (w *Interface) standHandler(writer http.ResponseWriter, request *http.Request) {
	err := w.corradeClient.StandUp()

	response := map[string]string{
		"status":  "success",
		"message": "Standing up",
	}

	if err != nil {
		response["status"] = "error"
		response["message"] = "Failed to stand up: " + err.Error()
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// setIdleBehaviorHandler marks a macro as idle behavior
func (w *Interface) setIdleBehaviorHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	macroName := vars["name"]

	if macroName == "" {
		http.Error(writer, "Macro name required", http.StatusBadRequest)
		return
	}

	// For web interface, use first owner as requestor
	requestor := "WebInterface"
	if len(w.config.Bot.Owners) > 0 {
		requestor = w.config.Bot.Owners[0]
	}

	err := w.chatProcessor.GetMacroManager().SetIdleBehavior(macroName, requestor, true)

	response := map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Macro '%s' marked as idle behavior", macroName),
	}

	if err != nil {
		response["status"] = "error"
		response["message"] = err.Error()
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// unsetIdleBehaviorHandler removes idle behavior marking from a macro
func (w *Interface) unsetIdleBehaviorHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	macroName := vars["name"]

	if macroName == "" {
		http.Error(writer, "Macro name required", http.StatusBadRequest)
		return
	}

	// For web interface, use first owner as requestor
	requestor := "WebInterface"
	if len(w.config.Bot.Owners) > 0 {
		requestor = w.config.Bot.Owners[0]
	}

	err := w.chatProcessor.GetMacroManager().SetIdleBehavior(macroName, requestor, false)

	response := map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Macro '%s' no longer an idle behavior", macroName),
	}

	if err != nil {
		response["status"] = "error"
		response["message"] = err.Error()
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// setAutoGreetMacroHandler marks a macro as auto-greet
func (w *Interface) setAutoGreetMacroHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	macroName := vars["name"]

	if macroName == "" {
		http.Error(writer, "Macro name required", http.StatusBadRequest)
		return
	}

	// For web interface, use first owner as requestor
	requestor := "WebInterface"
	if len(w.config.Bot.Owners) > 0 {
		requestor = w.config.Bot.Owners[0]
	}

	err := w.chatProcessor.GetMacroManager().SetAutoGreet(macroName, requestor, true)

	response := map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Macro '%s' marked as auto-greet macro", macroName),
	}

	if err != nil {
		response["status"] = "error"
		response["message"] = err.Error()
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

// unsetAutoGreetMacroHandler removes auto-greet marking from a macro
func (w *Interface) unsetAutoGreetMacroHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	macroName := vars["name"]

	if macroName == "" {
		http.Error(writer, "Macro name required", http.StatusBadRequest)
		return
	}

	// For web interface, use first owner as requestor
	requestor := "WebInterface"
	if len(w.config.Bot.Owners) > 0 {
		requestor = w.config.Bot.Owners[0]
	}

	err := w.chatProcessor.GetMacroManager().SetAutoGreet(macroName, requestor, false)

	response := map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Macro '%s' no longer an auto-greet macro", macroName),
	}

	if err != nil {
		response["status"] = "error"
		response["message"] = err.Error()
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}
