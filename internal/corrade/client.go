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
	config       config.CorradeConfig
	httpClient   *http.Client
	status       types.BotStatus
	botName      string // Store the bot's own name for position queries
	botUUID      string // Store the bot's UUID
	avatarsMutex sync.RWMutex
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
		botName: "", // Will be set when we have bot config
	}
}

// SetBotName sets the bot's name for position queries
func (c *Client) SetBotName(name string) {
	c.botName = name
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

	for key, value := range params {
		values.Set(key, value)
	}

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

// Tell makes the bot speak using the tell command (replaces Say)
func (c *Client) Tell(message string) error {
	params := map[string]string{
		"message": message,
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
		"x": fmt.Sprintf("%.2f", x),
		"y": fmt.Sprintf("%.2f", y),
		"z": fmt.Sprintf("%.2f", z),
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

// GetAvatarPosition gets an avatar's current position
func (c *Client) GetAvatarPosition(avatar string) (types.Position, error) {
	params := map[string]string{
		"firstname": strings.Split(avatar, " ")[0],
	}

	// Add lastname if available
	parts := strings.Split(avatar, " ")
	if len(parts) > 1 {
		params["lastname"] = parts[1]
	}

	response, err := c.sendCommand("getavatardata", params)
	if err != nil {
		return types.Position{}, err
	}

   log.Printf("getavatardata: %s", response)

	// Parse position from response
	pos := types.Position{}
	if strings.Contains(response, "Position") || strings.Contains(response, "GlobalPosition") {
		re := regexp.MustCompile(`(?:Position|GlobalPosition).*?(\d+(?:\.\d+)?).*?(\d+(?:\.\d+)?).*?(\d+(?:\.\d+)?)`)
		matches := re.FindStringSubmatch(response)
		if len(matches) >= 4 {
			fmt.Sscanf(matches[1], "%f", &pos.X)
			fmt.Sscanf(matches[2], "%f", &pos.Y)
			fmt.Sscanf(matches[3], "%f", &pos.Z)
		}
	}

	return pos, nil
}

// GetOwnPosition gets the bot's current position using getavatardata
func (c *Client) GetOwnPosition() types.Position {
	if c.botName == "" {
		log.Printf("Bot name not set, cannot get own position")
		return types.Position{}
	}

	// Split bot name into first and last name
	parts := strings.Split(c.botName, " ")
	params := map[string]string{
		"firstname": parts[0],
	}

	// Add lastname if available
	if len(parts) > 1 {
		params["lastname"] = parts[1]
	}

	response, err := c.sendCommand("getavatardata", params)
	if err != nil {
		log.Printf("Error getting own avatar data: %v", err)
		return types.Position{}
	}

	pos := types.Position{}
	// Try to parse position from response
	if strings.Contains(response, "Position") || strings.Contains(response, "GlobalPosition") {
		re := regexp.MustCompile(`(?:Position|GlobalPosition).*?(\d+(?:\.\d+)?).*?(\d+(?:\.\d+)?).*?(\d+(?:\.\d+)?)`)
		matches := re.FindStringSubmatch(response)
		if len(matches) >= 4 {
			fmt.Sscanf(matches[1], "%f", &pos.X)
			fmt.Sscanf(matches[2], "%f", &pos.Y)
			fmt.Sscanf(matches[3], "%f", &pos.Z)
		}
	}

	return pos
}

// GetNearbyAvatars gets avatars in the current region
func (c *Client) GetNearbyAvatars() (map[string]*types.AvatarInfo, error) {
	response, err := c.sendCommand("getmapavatarpositions", nil)
	if err != nil {
		return nil, err
	}

	c.avatarsMutex.Lock()
	defer c.avatarsMutex.Unlock()

	currentTime := time.Now()
	currentAvatars := make(map[string]string) // name -> uuid mapping for this scan

	// Parse avatar data from response
	// This regex should be adjusted based on the actual response format from Corrade
	avatarRegex := regexp.MustCompile(`FirstName.*?([^,\s]+).*?LastName.*?([^,\s]*).*?GlobalPosition.*?<(\d+(?:\.\d+)?),\s*(\d+(?:\.\d+)?),\s*(\d+(?:\.\d+)?)>.*?UUID.*?([a-fA-F0-9-]+)`)
	matches := avatarRegex.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) >= 7 {
			firstName := strings.Trim(match[1], `"`)
			lastName := strings.Trim(match[2], `"`)
			uuid := match[6]

			// Skip if this is the bot itself
			if firstName == strings.Split(c.botName, " ")[0] {
				continue
			}

			var x, y, z float64
			fmt.Sscanf(match[3], "%f", &x)
			fmt.Sscanf(match[4], "%f", &y)
			fmt.Sscanf(match[5], "%f", &z)

			name := firstName
			if lastName != "" && lastName != "Resident" {
				name += " " + lastName
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
				log.Printf("New avatar detected: %s", name)
			}
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

	// Return a copy of the current avatars
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

	return result, nil
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
		"data":   "Name",
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
