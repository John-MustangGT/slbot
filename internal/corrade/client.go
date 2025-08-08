package corrade

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"slbot/internal/config"
	"slbot/internal/types"
)

// Client handles all Corrade communication
type Client struct {
	config     config.CorradeConfig
	httpClient *http.Client
	status     types.BotStatus
}

// NewClient creates a new Corrade client
func NewClient(cfg config.CorradeConfig) *Client {
	return &Client{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		status: types.BotStatus{
			IsOnline:   false,
			LastUpdate: time.Now(),
		},
	}
}

// TestConnection tests the connection to Corrade
func (c *Client) TestConnection() error {
	_, err := c.sendCommand("getstatus", nil)
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

// Say makes the bot speak in Second Life
func (c *Client) Say(message string) error {
	params := map[string]string{
		"message": message,
	}
	_, err := c.sendCommand("say", params)
	return err
}

// Whisper makes the bot whisper to a specific avatar
func (c *Client) Whisper(avatar, message string) error {
	params := map[string]string{
		"avatar":  avatar,
		"message": message,
	}
	_, err := c.sendCommand("whisper", params)
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
		"avatar": avatar,
	}

	response, err := c.sendCommand("getavatardata", params)
	if err != nil {
		return types.Position{}, err
	}

	// Parse position from response (simplified - you may need more robust parsing)
	pos := types.Position{}
	if strings.Contains(response, "Position") {
		re := regexp.MustCompile(`Position.*?([0-9.]+).*?([0-9.]+).*?([0-9.]+)`)
		matches := re.FindStringSubmatch(response)
		if len(matches) >= 4 {
			fmt.Sscanf(matches[1], "%f", &pos.X)
			fmt.Sscanf(matches[2], "%f", &pos.Y)
			fmt.Sscanf(matches[3], "%f", &pos.Z)
		}
	}

	return pos, nil
}

// GetOwnPosition gets the bot's current position
func (c *Client) GetOwnPosition() types.Position {
	response, err := c.sendCommand("getposition", nil)
	if err != nil {
		return types.Position{}
	}

	pos := types.Position{}
	// Parse own position from response
	re := regexp.MustCompile(`([0-9.]+).*?([0-9.]+).*?([0-9.]+)`)
	matches := re.FindStringSubmatch(response)
	if len(matches) >= 4 {
		fmt.Sscanf(matches[1], "%f", &pos.X)
		fmt.Sscanf(matches[2], "%f", &pos.Y)
		fmt.Sscanf(matches[3], "%f", &pos.Z)
	}

	return pos
}

// GetCurrentRegion gets the current region/sim name
func (c *Client) GetCurrentRegion() string {
	response, err := c.sendCommand("getregiondata", nil)
	if err != nil {
		return "Unknown"
	}

	// Parse region name from response (simplified)
	if strings.Contains(response, "RegionName") {
		re := regexp.MustCompile(`RegionName.*?"([^"]+)"`)
		matches := re.FindStringSubmatch(response)
		if len(matches) >= 2 {
			return matches[1]
		}
	}
	return "Unknown"
}

// GetEvents gets events from Corrade
func (c *Client) GetEvents() (string, error) {
	return c.sendCommand("getevent", map[string]string{
		"callback": "InstantMessage,LocalChat,RegionSayDistance",
	})
}

// ParseEvents parses event responses from Corrade
func (c *Client) ParseEvents(response string) []types.ChatMessage {
	var messages []types.ChatMessage

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		if strings.Contains(line, "LocalChat") || strings.Contains(line, "InstantMessage") {
			// Extract chat information using regex
			re := regexp.MustCompile(`"([^"]+)","([^"]+)","([^"]+)"`)
			matches := re.FindStringSubmatch(line)

			if len(matches) >= 4 {
				message := types.ChatMessage{
					Avatar:  matches[1],
					Message: matches[2],
					UUID:    matches[3],
					Type:    "chat",
				}
				messages = append(messages, message)
			}
		}
	}

	return messages
}

// UpdateStatus updates the bot's current status
func (c *Client) UpdateStatus() types.BotStatus {
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
	return c.status
}

// SetFollowing sets the following status
func (c *Client) SetFollowing(following bool, target string) {
	c.status.IsFollowing = following
	c.status.FollowTarget = target
}

// CalculateDistance calculates 3D distance between two positions
func CalculateDistance(pos1, pos2 types.Position) float64 {
	dx := pos1.X - pos2.X
	dy := pos1.Y - pos2.Y
	dz := pos1.Z - pos2.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
