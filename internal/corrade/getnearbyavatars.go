package corrade

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"slbot/internal/types"
)

// GetNearbyAvatars requests avatars in the current region using getmapavatarpositions
// This is an async operation that will update the cache via callback
func (c *Client) GetNearbyAvatars() (map[string]*types.AvatarInfo, error) {
	// Return current cached data immediately
	return c.getCachedAvatars(), nil
}

// RequestNearbyAvatars initiates an async request for nearby avatars
func (c *Client) RequestNearbyAvatars(callbackURL string) error {
	// Get current region name
	region := c.GetCurrentRegion()
	if region == "Unknown" {
		return fmt.Errorf("cannot determine current region")
	}

	// Send async command with region parameter and callback URL
	params := map[string]string{
		"region":   region,
		"callback": callbackURL,
	}
	_, err := c.sendCommand("getmapavatarpositions", params)
	return err
}

// ProcessMapAvatarPositionsCallback processes the callback from getmapavatarpositions
func (c *Client) ProcessMapAvatarPositionsCallback(data map[string]interface{}) {
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
		log.Printf("No avatar data in callback")
		return
	}

	// Parse the avatar data
	// Format: count,uuid1,"<x1,y1,z1>",uuid2,"<x2,y2,z2>",...
	parts := strings.Split(avatarData, ",")
	
	if len(parts) < 1 {
		log.Printf("Invalid avatar data format")
		return
	}

	// First part should be count
	count := 0
	if len(parts) >= 1 {
		fmt.Sscanf(parts[0], "%d", &count)
	}

	log.Printf("Processing %d avatars from callback", count)

	// Process avatar data in pairs: UUID, Position
	for i := 1; i < len(parts) && i+1 < len(parts); i += 2 {
		uuid := strings.Trim(parts[i], " \"")
		positionStr := ""
		
		// Position might be quoted and span multiple comma-separated parts
		if i+1 < len(parts) {
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
			}
		}
		
		positionStr = strings.Trim(positionStr, " \"")

		// Parse position from format "<x, y, z>" or "<x,+y,+z>"
		var x, y, z float64
		posRegex := regexp.MustCompile(`<(\d+(?:\.\d+)?),\s*[+]?(\d+(?:\.\d+)?),\s*[+]?(\d+(?:\.\d+)?)>`)
		posMatches := posRegex.FindStringSubmatch(positionStr)
		if len(posMatches) >= 4 {
			fmt.Sscanf(posMatches[1], "%f", &x)
			fmt.Sscanf(posMatches[2], "%f", &y)
			fmt.Sscanf(posMatches[3], "%f", &z)
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
	c.uuidNameMap[uuid] = name
}

// UpdateAvatarName updates an avatar's name when we learn it from other sources (like chat)
func (c *Client) UpdateAvatarName(uuid, name string) {
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
