package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"slbot/internal/chat"
	"slbot/internal/config"
	"slbot/internal/corrade"
	"slbot/internal/types"
)

// Interface handles the web dashboard
type Interface struct {
	config        *config.Config
	corradeClient *corrade.Client
	chatProcessor *chat.Processor
	server        *http.Server
	templates     *template.Template
}

// NewInterface creates a new web interface
func NewInterface(cfg *config.Config, corradeClient *corrade.Client, chatProcessor *chat.Processor) *Interface {
	return &Interface{
		config:        cfg,
		corradeClient: corradeClient,
		chatProcessor: chatProcessor,
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
	api.HandleFunc("/logs", w.logsHandler).Methods("GET")
	api.HandleFunc("/teleport", w.teleportHandler).Methods("POST")
	api.HandleFunc("/walk", w.walkHandler).Methods("POST")
	api.HandleFunc("/stop-following", w.stopFollowingHandler).Methods("POST")
	api.HandleFunc("/stand", w.standHandler).Methods("POST")
	api.HandleFunc("/toggle-llama", w.toggleLlamaHandler).Methods("POST")
	
	// Macro API endpoints
	macroAPI := api.PathPrefix("/macros").Subrouter()
	macroAPI.HandleFunc("", w.getMacrosHandler).Methods("GET")
	macroAPI.HandleFunc("/play/{name}", w.playMacroHandler).Methods("POST")
	macroAPI.HandleFunc("/delete/{name}", w.deleteMacroHandler).Methods("DELETE")
	macroAPI.HandleFunc("/recording", w.getRecordingStatusHandler).Methods("GET")
	macroAPI.HandleFunc("/idle/{name}", w.setIdleBehaviorHandler).Methods("POST")
	macroAPI.HandleFunc("/idle/{name}", w.unsetIdleBehaviorHandler).Methods("DELETE")

	// Create server
	w.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", w.config.Bot.WebPort),
		Handler: router,
	}

	log.Printf("Web interface starting on http://localhost:%d", w.config.Bot.WebPort)

	// Start periodic status updates
	go w.statusUpdateRoutine(ctx)

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

// corradeNotificationHandler handles notifications from Corrade
func (w *Interface) corradeNotificationHandler(writer http.ResponseWriter, request *http.Request) {
	var notification map[string]interface{}
	
	if err := json.NewDecoder(request.Body).Decode(&notification); err != nil {
		log.Printf("Error decoding Corrade notification: %v", err)
		http.Error(writer, "Bad Request", http.StatusBadRequest)
		return
	}

	// Process the notification through the chat processor
	w.chatProcessor.ProcessNotification(notification)

	// Respond with success
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("OK"))
}

// loadTemplates loads HTML templates
func (w *Interface) loadTemplates() error {
	templatePath := filepath.Join("web", "templates", "*.html")
	templates, err := template.New("").Funcs(template.FuncMap{
		"add": func(a, b int) int {
			return a + b
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
	// Note: pendingSit functionality simplified - always returns nil now
	pendingSit := (*types.PendingSitConfirmation)(nil)

	data := struct {
		Status          types.BotStatus
		Logs            []types.LogEntry
		LlamaEnabled    bool
		Macros          map[string]*types.Macro
		IsRecording     bool
		RecordingStatus *types.MacroRecording
		IsIdle          bool
		PendingSit      *types.PendingSitConfirmation
	}{
		Status:          status,
		Logs:            logs,
		LlamaEnabled:    w.chatProcessor.IsLlamaEnabled(),
		Macros:          macros,
		IsRecording:     recordingStatus != nil,
		RecordingStatus: recordingStatus,
		IsIdle:          isIdle,
		PendingSit:      pendingSit,
	}

	if err := w.templates.ExecuteTemplate(writer, "dashboard.html", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
	}
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
	// In a real implementation, you'd want proper authentication
	requestor := "WebInterface"
	if len(w.config.Bot.Owners) > 0 {
		requestor = w.config.Bot.Owners[0]
	}
	
	err := w.chatProcessor.GetMacroManager().PlayMacro(macroName, requestor)
	
	response := map[string]string{
		"status": "success",
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
		"status": "success",
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
		"status": "success",
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
		"status": "success",
		"message": fmt.Sprintf("Macro '%s' no longer an idle behavior", macroName),
	}

	if err != nil {
		response["status"] = "error"
		response["message"] = err.Error()
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}
