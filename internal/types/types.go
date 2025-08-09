package types

import "time"

// Position represents 3D coordinates
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// ChatMessage represents a chat message from Second Life
type ChatMessage struct {
	Avatar   string   `json:"avatar"`
	Message  string   `json:"message"`
	UUID     string   `json:"uuid"`
	Type     string   `json:"type"`
	Position Position `json:"position"`
}

// FollowTarget represents an avatar being followed
type FollowTarget struct {
	Avatar   string    `json:"avatar"`
	UUID     string    `json:"uuid"`
	LastSeen time.Time `json:"lastSeen"`
	Position Position  `json:"position"`
}

// AvatarInfo represents an avatar in the region
type AvatarInfo struct {
	Name     string    `json:"name"`
	UUID     string    `json:"uuid"`
	Position Position  `json:"position"`
	LastSeen time.Time `json:"lastSeen"`
	FirstSeen time.Time `json:"firstSeen"`
	IsGreeted bool      `json:"isGreeted"`
}

// BotStatus represents current bot status
type BotStatus struct {
	IsOnline                bool                   `json:"isOnline"`
	CurrentSim              string                 `json:"currentSim"`
	Position                Position               `json:"position"`
	IsFollowing             bool                   `json:"isFollowing"`
	FollowTarget            string                 `json:"followTarget"`
	IsSitting               bool                   `json:"isSitting"`
	SitObject               string                 `json:"sitObject"`
	LastUpdate              time.Time              `json:"lastUpdate"`
	IdleBehaviorMinInterval int                    `json:"idleBehaviorMinInterval"`
	IdleBehaviorMaxInterval int                    `json:"idleBehaviorMaxInterval"`
	NearbyAvatars           map[string]*AvatarInfo `json:"nearbyAvatars"`
	AutoGreetEnabled        bool                   `json:"autoGreetEnabled"`
	AutoGreetMacro          string                 `json:"autoGreetMacro,omitempty"`
}

// LogEntry represents a chat or system log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "chat", "im", "system", "movement", "avatar"
	Avatar    string    `json:"avatar"`
	Message   string    `json:"message"`
	Response  string    `json:"response,omitempty"`
}

// TeleportRequest represents a teleport request from web interface
type TeleportRequest struct {
	Region string  `json:"region"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Z      float64 `json:"z"`
}

// WalkRequest represents a walk request from web interface
type WalkRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// PendingSitConfirmation represents a pending sit confirmation request
type PendingSitConfirmation struct {
	Avatar      string                  `json:"avatar"`
	SearchTerm  string                  `json:"searchTerm"`
	Objects     []NearbyObject          `json:"objects"`
	RequestTime time.Time               `json:"requestTime"`
	Timeout     time.Duration           `json:"timeout"`
}

// NearbyObject represents an object found near the bot
type NearbyObject struct {
	Name     string  `json:"name"`
	UUID     string  `json:"uuid"`
	Distance float64 `json:"distance"`
}

// MacroAction represents a single recorded action
type MacroAction struct {
	Type      string                 `json:"type"`      // "walk", "teleport", "sit", "stand", "tell", "wait", "whisper"
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// Macro represents a sequence of recorded actions
type Macro struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Actions      []MacroAction `json:"actions"`
	CreatedBy    string        `json:"createdBy"`
	CreatedAt    time.Time     `json:"createdAt"`
	Duration     time.Duration `json:"duration"`
	Tags         []string      `json:"tags"`         // Tags for categorizing macros
	IdleBehavior bool          `json:"idleBehavior"` // Mark as idle behavior
	AutoGreet    bool          `json:"autoGreet"`    // Mark as auto-greet macro
}

// MacroRecording represents an active recording session
type MacroRecording struct {
	Name        string        `json:"name"`
	StartTime   time.Time     `json:"startTime"`
	Actions     []MacroAction `json:"actions"`
	RecordedBy  string        `json:"recordedBy"`
	IsRecording bool          `json:"isRecording"`
}

// LlamaRequest represents request to Llama API
type LlamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// LlamaResponse represents response from Llama API
type LlamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// AutoGreetRequest represents an auto-greet configuration request
type AutoGreetRequest struct {
	Enabled   bool   `json:"enabled"`
	MacroName string `json:"macroName,omitempty"`
}
