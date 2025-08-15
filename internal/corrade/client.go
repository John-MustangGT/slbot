package corrade

import (
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"slbot/internal/config"
	"slbot/internal/types"
)

// Client handles all Corrade communication
type Client struct {
	config           config.CorradeConfig
	httpClient       *http.Client
	status           types.BotStatus
	botName          string // Store the bot's own name for position queries
	botUUID          string // Store the bot's UUID
	avatarsMutex     sync.RWMutex
	pendingRequests  map[string]chan types.Position // For async position requests
	requestsMutex    sync.RWMutex
	uuidNameMap      map[string]string // UUID to name mapping
	nameMapMutex     sync.RWMutex
}

// NewClient creates a new Corrade client
func NewClient(cfg config.CorradeConfig) *Client {
	return &Client{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		status: types.BotStatus{
			IsOnline:      false,
			LastUpdate:    time.Now(),
			NearbyAvatars: make(map[string]*types.AvatarInfo),
		},
		botName:         "", // Will be set when we have bot config
		botUUID:         "", // Will be set when we discover it
		pendingRequests: make(map[string]chan types.Position),
		uuidNameMap:     make(map[string]string), // Initialize the UUID name mapping
	}
}

// SetBotName sets the bot's name for position queries
func (c *Client) SetBotName(name string) {
	c.botName = name
}

// SetBotUUID sets the bot's UUID (NEW)
func (c *Client) SetBotUUID(uuid string) {
	c.botUUID = uuid
}

// TestConnection tests the connection to Corrade
func (c *Client) TestConnection() error {
	// Use getregiondata as a test since it's a known valid command
	_, err := c.sendCommand("getregiondata", nil)
	return err
}

// sendCommand sends a command to Corrade
func (c *Client) sendCommand(command string, params map[string]string) (string, error) {
	values := url.Values{}
	values.Set("command", command)
	values.Set("group", c.config.Group)
	values.Set("password", c.config.Password)

   //log.Printf("Command=%s params=%q", command, params)

	for key, value := range params {
		values.Set(key, value)
	}

   //log.Printf("Request= %s\n", formatURLValues(values))

	resp, err := c.httpClient.PostForm(c.config.URL, values)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// SetupNotification sets up a notification for specific events
func (c *Client) SetupNotification(eventType, callbackURL string) error {
	params := map[string]string{
		"action": "add",
		"type":   eventType,
		"URL":    callbackURL,
	}
	response, err := c.sendCommand("notify", params)
	if err != nil {
		return err
	}

	if !strings.Contains(response, "success") {
		return fmt.Errorf("failed to setup notification for %s: %s", eventType, response)
	}

	log.Printf("Setup notification for %s to %s", eventType, callbackURL)
	return nil
}

// RequestAvatarData requests avatar data for all avatars in the region
// This will trigger callbacks with avatar information
func (c *Client) RequestAvatarData(region string, callbackURL string) error {
	params := map[string]string{
      "entity":   "parcel",
		"region":   region,
		"callback": callbackURL,
	}
	_, err := c.sendCommand("getavatarpositions", params)
	return err
}

// ProcessAvatarDataCallback processes the callback from getavatardata
func (c *Client) ProcessAvatarDataCallback(data map[string]interface{}) {
	c.avatarsMutex.Lock()
	defer c.avatarsMutex.Unlock()

	// Extract avatar information from callback data
	// This is a simplified version - you'll need to adjust based on actual callback format
	if firstName, ok := data["FirstName"].(string); ok {
		lastName := ""
		if ln, exists := data["LastName"].(string); exists {
			lastName = ln
		}
		
		name := firstName
		if lastName != "" && lastName != "Resident" {
			name += " " + lastName
		}

		// Skip if this is the bot itself
		if firstName == strings.Split(c.botName, " ")[0] {
			return
		}

		currentTime := time.Now()
		
		// Extract position if available
		var pos types.Position
		if posData, exists := data["GlobalPosition"].(string); exists {
			// Parse position string format like "<x, y, z>"
			re := regexp.MustCompile(`<(\d+(?:\.\d+)?),\s*(\d+(?:\.\d+)?),\s*(\d+(?:\.\d+)?)>`)
			matches := re.FindStringSubmatch(posData)
			if len(matches) >= 4 {
				fmt.Sscanf(matches[1], "%f", &pos.X)
				fmt.Sscanf(matches[2], "%f", &pos.Y)
				fmt.Sscanf(matches[3], "%f", &pos.Z)
			}
		}

		uuid := ""
		if u, exists := data["UUID"].(string); exists {
			uuid = u
		}

		if existingAvatar, exists := c.status.NearbyAvatars[name]; exists {
			// Update existing avatar
			existingAvatar.Position = pos
			existingAvatar.LastSeen = currentTime
			existingAvatar.UUID = uuid
		} else {
			// New avatar
			c.status.NearbyAvatars[name] = &types.AvatarInfo{
				Name:      name,
				UUID:      uuid,
				Position:  pos,
				FirstSeen: currentTime,
				LastSeen:  currentTime,
				IsGreeted: false,
			}
			log.Printf("New avatar detected: %s at position (%.2f, %.2f, %.2f)", name, pos.X, pos.Y, pos.Z)
		}

		// Check if there's a pending request for this avatar
		c.requestsMutex.Lock()
		if ch, exists := c.pendingRequests[name]; exists {
			select {
			case ch <- pos:
			default:
			}
			delete(c.pendingRequests, name)
		}
		c.requestsMutex.Unlock()

		// Update name mapping (NEW)
		if uuid != "" {
			c.setNameForUUID(uuid, name)
		}
	}
}

// Tell makes the bot speak using the tell command in local/channel 0 (replaces Say)
func (c *Client) Tell(message string) error {
	params := map[string]string{
		"message": message,
		"entity":  "local",
		"type":    "Normal",
	}
	_, err := c.sendCommand("tell", params)
	return err
}

// Tell makes the bot speak using the tell command (replaces Say)
func (c *Client) TellChannel(channel int, message string) error {
	params := map[string]string{
		"message": message,
      "channel": fmt.Sprintf("%d", channel),
		"entity":  "local",
		"type":    "Normal",
	}
	_, err := c.sendCommand("tell", params)
	return err
}

// Whisper makes the bot whisper to a specific avatar using tell command
func (c *Client) Whisper(avatar, message string) error {
	params := map[string]string{
		"agent":   avatar,
		"message": message,
		"entity":  "avatar",
		"type":    "Whisper",
	}
	_, err := c.sendCommand("tell", params)
	return err
}

// WalkTo moves the bot to specific coordinates
func (c *Client) WalkTo(x, y, z float64) error {
	params := map[string]string{
		"position": fmt.Sprintf("<%.2f,%.2f,%.2f>", x, y, z),
      "action": "start",
	}
	_, err := c.sendCommand("walkto", params)
	return err
}

// Teleport teleports the bot to a location
func (c *Client) Teleport(region string, x, y, z float64) error {
	params := map[string]string{
		"region": region,
		"x":      fmt.Sprintf("%.0f", x),
		"y":      fmt.Sprintf("%.0f", y),
		"z":      fmt.Sprintf("%.0f", z),
	}
	_, err := c.sendCommand("teleport", params)
	return err
}

// SitOn makes the bot sit on a specific object
func (c *Client) SitOn(objectName string) error {
	params := map[string]string{
		"item": objectName,
	}
   log.Printf("Sit Target: %s", objectName)
	response, err := c.sendCommand("sit", params)
	if err == nil && strings.Contains(response, "success") {
		c.status.IsSitting = true
		c.status.SitObject = objectName
	}
	return err
}

// StandUp makes the bot stand up
func (c *Client) StandUp() error {
	_, err := c.sendCommand("stand", nil)
	if err == nil {
		c.status.IsSitting = false
		c.status.SitObject = ""
	}
	return err
}

// GetAvatarPosition gets an avatar's current position from cached data
// If not in cache, returns last known position or zero position
func (c *Client) GetAvatarPosition(avatar string) (types.Position, error) {
	c.avatarsMutex.RLock()
	defer c.avatarsMutex.RUnlock()

	if avatarInfo, exists := c.status.NearbyAvatars[avatar]; exists {
		return avatarInfo.Position, nil
	}

	return types.Position{}, fmt.Errorf("avatar %s not found in cached data", avatar)
}

// GetAvatarPositionAsync requests an avatar's position asynchronously
// Returns a channel that will receive the position when available
func (c *Client) GetAvatarPositionAsync(avatar string, timeout time.Duration) (<-chan types.Position, error) {
	c.requestsMutex.Lock()
	defer c.requestsMutex.Unlock()

	// Check if already in cache
	c.avatarsMutex.RLock()
	if avatarInfo, exists := c.status.NearbyAvatars[avatar]; exists {
		c.avatarsMutex.RUnlock()
		ch := make(chan types.Position, 1)
		ch <- avatarInfo.Position
		close(ch)
		return ch, nil
	}
	c.avatarsMutex.RUnlock()

	// Create channel for async response
	ch := make(chan types.Position, 1)
	c.pendingRequests[avatar] = ch

	// Request avatar data for current region
	region := c.GetCurrentRegion()
	if region == "Unknown" {
		delete(c.pendingRequests, avatar)
		return nil, fmt.Errorf("cannot determine current region")
	}

	// This would need a callback URL setup
	// For now, we'll just trigger a region scan
	go func() {
		time.Sleep(timeout)
		c.requestsMutex.Lock()
		if ch, exists := c.pendingRequests[avatar]; exists {
			close(ch)
			delete(c.pendingRequests, avatar)
		}
		c.requestsMutex.Unlock()
	}()

	return ch, nil
}

// GetOwnPosition gets the bot's current position using getavatardata
func (c *Client) GetOwnPosition() types.Position {
   return c.status.Position
}

// GetNewAvatars returns avatars that haven't been greeted yet
func (c *Client) GetNewAvatars() []*types.AvatarInfo {
	c.avatarsMutex.RLock()
	defer c.avatarsMutex.RUnlock()

	var newAvatars []*types.AvatarInfo
	for _, avatar := range c.status.NearbyAvatars {
		if !avatar.IsGreeted {
			newAvatars = append(newAvatars, avatar)
		}
	}

	return newAvatars
}

// MarkAvatarGreeted marks an avatar as having been greeted
func (c *Client) MarkAvatarGreeted(name string) {
	c.avatarsMutex.Lock()
	defer c.avatarsMutex.Unlock()

	if avatar, exists := c.status.NearbyAvatars[name]; exists {
		avatar.IsGreeted = true
	}
}

// GetCurrentRegion gets the current region/sim name
func (c *Client) GetCurrentRegion() string {
	params := map[string]string{
		"data": "Name",
	}
	response, err := c.sendCommand("getregiondata", params)
	if err != nil {
		return "Unknown"
	}

	answers, err := url.ParseQuery(response)
	if err != nil {
		return "Unknown"
	}
	if answers.Has("data") {
		data := strings.Split(answers.Get("data"), ",")
		if data[0] == "Name" {
			return data[1]
		}
	}
	return "Unknown"
}

// UpdateStatus updates the bot's current status
func (c *Client) UpdateStatus() types.BotStatus {
	// Get position using the corrected method
	pos := c.GetOwnPosition()
	region := c.GetCurrentRegion()

	c.status.IsOnline = true
	c.status.CurrentSim = region
	c.status.Position = pos
	c.status.LastUpdate = time.Now()

	return c.status
}

// UpdateStatusWithConfig updates the bot's status including configuration
func (c *Client) UpdateStatusWithConfig(config interface{}) types.BotStatus {
	status := c.UpdateStatus()

	// Add configuration data if provided
	if cfg, ok := config.(interface {
		GetIdleBehaviorMinInterval() int
		GetIdleBehaviorMaxInterval() int
	}); ok {
		status.IdleBehaviorMinInterval = cfg.GetIdleBehaviorMinInterval()
		status.IdleBehaviorMaxInterval = cfg.GetIdleBehaviorMaxInterval()
	}

	return status
}

// GetStatus returns the current bot status
func (c *Client) GetStatus() types.BotStatus {
	// Make a copy to prevent external modification of NearbyAvatars
	statusCopy := c.status
	statusCopy.NearbyAvatars = make(map[string]*types.AvatarInfo)

	c.avatarsMutex.RLock()
	for name, avatar := range c.status.NearbyAvatars {
		statusCopy.NearbyAvatars[name] = &types.AvatarInfo{
			Name:      avatar.Name,
			UUID:      avatar.UUID,
			Position:  avatar.Position,
			FirstSeen: avatar.FirstSeen,
			LastSeen:  avatar.LastSeen,
			IsGreeted: avatar.IsGreeted,
		}
	}
	c.avatarsMutex.RUnlock()

	return statusCopy
}

// SetFollowing sets the following status
func (c *Client) SetFollowing(following bool, target string) {
	c.status.IsFollowing = following
	c.status.FollowTarget = target
}

// SetAutoGreet sets the auto-greet configuration
func (c *Client) SetAutoGreet(enabled bool, macroName string) {
	c.status.AutoGreetEnabled = enabled
	c.status.AutoGreetMacro = macroName
}

// GetAutoGreetConfig returns the current auto-greet configuration
func (c *Client) GetAutoGreetConfig() (bool, string) {
	return c.status.AutoGreetEnabled, c.status.AutoGreetMacro
}

// CalculateDistance calculates 3D distance between two positions
func CalculateDistance(pos1, pos2 types.Position) float64 {
	dx := pos1.X - pos2.X
	dy := pos1.Y - pos2.Y
	dz := pos1.Z - pos2.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// GetNearbyAvatars requests avatars in the current region using getmapavatarpositions
// This is an async operation that will update the cache via callback
func (c *Client) GetNearbyAvatars() (map[string]*types.AvatarInfo, error) {
	// Return current cached data immediately
	return c.getCachedAvatars(), nil
}

// RequestNearbyAvatars initiates an async request for nearby avatars (UPDATED)
func (c *Client) RequestNearbyAvatars(callbackURL string) error {
	// Get current region name
	region := c.GetCurrentRegion()
	if region == "Unknown" {
		return fmt.Errorf("cannot determine current region")
	}

	// Send async command with region parameter and callback URL
	params := map[string]string{
		"region":   region,
      "entity":  "parcel",
		"callback": callbackURL,
	}

	log.Printf("Requesting nearby avatars for region: %s with callback: %s", region, callbackURL)
	_, err := c.sendCommand("getavatarpositions", params)
	return err
}

// ProcessMapAvatarPositionsCallback processes the callback from getmapavatarpositions (ENHANCED)
func (c *Client) ProcessMapAvatarPositionsCallback(data map[string]interface{}) {
   log.Printf("Map AvatarPositions.data: %q\n", data)
	c.avatarsMutex.Lock()
	defer c.avatarsMutex.Unlock()

	currentTime := time.Now()
	currentAvatars := make(map[string]string) // name -> uuid mapping for this scan

	// Check if the request was successful
	if success, ok := data["success"].(string); ok && success != "True" {
		log.Printf("getmapavatarpositions callback failed: %v", data)
		return
	}

	// Get the data from callback
	avatarData, exists := data["data"].(string)
	if !exists {
		log.Printf("No avatar data in callback: %+v", data)
		return
	}

	// Parse the avatar data
	// Format: count,uuid1,"<x1,y1,z1>",uuid2,"<x2,y2,z2>",...
	parts := strings.Split(avatarData, ",")
	
	if len(parts) < 1 {
		log.Printf("Invalid avatar data format: %s", avatarData)
		return
	}

	// First part should be count
	count := 0
	if len(parts) >= 1 {
		fmt.Sscanf(parts[0], "%d", &count)
	}

	log.Printf("Processing %d avatars from callback", count)

	// Process avatar data in pairs: UUID, Position
	i := 1
	for i < len(parts) {
		if i+1 >= len(parts) {
			break
		}

		uuid := strings.Trim(parts[i], " \"")
		positionStr := ""
		
		// Position might be quoted and span multiple comma-separated parts
		positionStr = parts[i+1]
		// If position starts with quote but doesn't end with quote, collect more parts
		if strings.HasPrefix(positionStr, "\"") && !strings.HasSuffix(positionStr, "\"") {
			for j := i + 2; j < len(parts); j++ {
				positionStr += "," + parts[j]
				if strings.HasSuffix(parts[j], "\"") {
					i = j // Update index to skip processed parts
					break
				}
			}
		} else {
			i++ // Normal increment
		}
		i++ // Move to next pair
		
		positionStr = strings.Trim(positionStr, " \"")

		// Parse position from format "<x, y, z>" or "<x,+y,+z>"
		var x, y, z float64
		posRegex := regexp.MustCompile(`<([+-]?\d+(?:\.\d+)?),\s*([+-]?\d+(?:\.\d+)?),\s*([+-]?\d+(?:\.\d+)?)>`)
		posMatches := posRegex.FindStringSubmatch(positionStr)
		if len(posMatches) >= 4 {
			fmt.Sscanf(posMatches[1], "%f", &x)
			fmt.Sscanf(posMatches[2], "%f", &y)
			fmt.Sscanf(posMatches[3], "%f", &z)
		} else {
			log.Printf("Could not parse position: %s", positionStr)
			continue
		}

		// Skip if this is the bot itself
		if uuid == c.botUUID {
			continue
		}

		// Check if we have a name mapping for this UUID
		name := c.getNameForUUID(uuid)
		if name == "" {
			// Generate temporary name
			uuidShort := uuid
			if len(uuid) > 8 {
				uuidShort = uuid[:8]
			}
			name = fmt.Sprintf("Avatar-%s", uuidShort)
		}

		currentAvatars[name] = uuid
		pos := types.Position{X: x, Y: y, Z: z}

		if existingAvatar, exists := c.status.NearbyAvatars[name]; exists {
			// Update existing avatar
			existingAvatar.Position = pos
			existingAvatar.LastSeen = currentTime
			existingAvatar.UUID = uuid
		} else {
			// New avatar
			c.status.NearbyAvatars[name] = &types.AvatarInfo{
				Name:      name,
				UUID:      uuid,
				Position:  pos,
				FirstSeen: currentTime,
				LastSeen:  currentTime,
				IsGreeted: false,
			}
			log.Printf("New avatar detected: %s (UUID: %s) at position (%.2f, %.2f, %.2f)", name, uuid, x, y, z)
		}
	}

	// Remove avatars that are no longer in the region (not seen for 2 minutes)
	for name, avatar := range c.status.NearbyAvatars {
		if _, stillPresent := currentAvatars[name]; !stillPresent {
			if time.Since(avatar.LastSeen) > 2*time.Minute {
				delete(c.status.NearbyAvatars, name)
				log.Printf("Avatar left region: %s", name)
			}
		}
	}
}

// getNameForUUID attempts to find a real name for a UUID from various sources
func (c *Client) getNameForUUID(uuid string) string {
	// First check the UUID-to-name mapping
	c.nameMapMutex.RLock()
	if name, exists := c.uuidNameMap[uuid]; exists {
		c.nameMapMutex.RUnlock()
		return name
	}
	c.nameMapMutex.RUnlock()
	
	// Then check if we already have this UUID with a real name in nearby avatars
	for _, avatar := range c.status.NearbyAvatars {
		if avatar.UUID == uuid && !strings.HasPrefix(avatar.Name, "Avatar-") {
			// Cache this mapping
			c.setNameForUUID(uuid, avatar.Name)
			return avatar.Name
		}
	}
	
	// Could also check other sources like recent chat logs, etc.
	// For now, return empty to use temporary name
	return ""
}

// setNameForUUID stores a UUID-to-name mapping
func (c *Client) setNameForUUID(uuid, name string) {
	c.nameMapMutex.Lock()
	defer c.nameMapMutex.Unlock()
	if c.uuidNameMap == nil {
		c.uuidNameMap = make(map[string]string)
	}
	c.uuidNameMap[uuid] = name
}

// UpdateAvatarName updates an avatar's name when we learn it from other sources (like chat) (ENHANCED)
func (c *Client) UpdateAvatarName(uuid, name string) {
	if uuid == "" || name == "" {
		return
	}
	
	c.setNameForUUID(uuid, name)
	
	c.avatarsMutex.Lock()
	defer c.avatarsMutex.Unlock()
	
	// Find and update the avatar in our cache
	var oldName string
	for avatarName, avatar := range c.status.NearbyAvatars {
		if avatar.UUID == uuid {
			oldName = avatarName
			break
		}
	}
	
	// If we found the avatar with the old temporary name, update it
	if oldName != "" && oldName != name {
		avatar := c.status.NearbyAvatars[oldName]
		avatar.Name = name
		c.status.NearbyAvatars[name] = avatar
		delete(c.status.NearbyAvatars, oldName)
		log.Printf("Updated avatar name from %s to %s (UUID: %s)", oldName, name, uuid)
	}
}

// getCachedAvatars returns a copy of cached avatar data
func (c *Client) getCachedAvatars() map[string]*types.AvatarInfo {
	c.avatarsMutex.RLock()
	defer c.avatarsMutex.RUnlock()
	
	result := make(map[string]*types.AvatarInfo)
	for name, avatar := range c.status.NearbyAvatars {
		result[name] = &types.AvatarInfo{
			Name:      avatar.Name,
			UUID:      avatar.UUID,
			Position:  avatar.Position,
			FirstSeen: avatar.FirstSeen,
			LastSeen:  avatar.LastSeen,
			IsGreeted: avatar.IsGreeted,
		}
	}
	return result
}
