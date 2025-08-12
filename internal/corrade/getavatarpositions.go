package corrade

import (
   "time"
   "log"
   "strings"
   "fmt"
   "regexp"
   "encoding/csv"
   "slbot/internal/types"
)

func (c *Client) ProcessGetAvatarPositionsCallback(data map[string]interface{}) {

   // Check if the request was successful
   if success, ok := data["success"].(string); ok && success != "True" {
      log.Printf("getmapavatarpositions callback failed: %v", data)
      return
   }

   c.avatarsMutex.Lock()
   defer c.avatarsMutex.Unlock()

   currentTime := time.Now()
   if dataTime, ok := data["time"].(string); ok {
      parsedTime, err := time.Parse(time.RFC3339, dataTime)
      if err != nil {
         log.Printf("time.Parse Error: %s", err)
      } else {
         currentTime = parsedTime
      }
   }

   // Get the data from callback
   avatarData, exists := data["data"].(string)
   if !exists {
      log.Printf("No avatar data in callback: %+v", data)
      return
   }

   r := csv.NewReader(strings.NewReader(avatarData))
   parts, _ := r.Read()
   if len(parts) < 3 {
      return
   }

   for i:=0; i<len(parts); i+=3 {
      thisAvatar := &types.AvatarInfo{
         Name:      normalizeName(parts[i]),
         UUID:      parts[i+1],
         Position:  parsePositionString(strings.Trim(parts[i+2], "\"")),
         FirstSeen: currentTime,
         LastSeen:  currentTime,
         IsGreeted: false,
      }

      // Skip if this is the bot itself
      if thisAvatar.UUID == c.botUUID || matchName(thisAvatar.Name, c.botName) {
         c.SetBotUUID(thisAvatar.UUID)
         c.status.Position = thisAvatar.Position
         c.status.LastUpdate = currentTime
         continue
      }

      if existingAvatar, exists := c.status.NearbyAvatars[thisAvatar.Name]; exists {
         // Update existing avatar
         existingAvatar.Name = thisAvatar.Name
         existingAvatar.UUID = thisAvatar.UUID
         existingAvatar.Position = thisAvatar.Position
         existingAvatar.LastSeen = thisAvatar.LastSeen
      } else {
         // New avatar
         c.status.NearbyAvatars[thisAvatar.Name] = thisAvatar
      }
   }

   c.cleanupAvatars()
}

func parsePositionString(pos string) types.Position {
      var x, y, z float64

      posRegex := regexp.MustCompile(`<([+-]?\d+(?:\.\d+)?),\s*([+-]?\d+(?:\.\d+)?),\s*([+-]?\d+(?:\.\d+)?)>`)
      posMatches := posRegex.FindStringSubmatch(pos)
      if len(posMatches) >= 4 {
         fmt.Sscanf(posMatches[1], "%f", &x)
         fmt.Sscanf(posMatches[2], "%f", &y)
         fmt.Sscanf(posMatches[3], "%f", &z)
      } else {
         log.Printf("Could not parse position: %s", pos)
      }
      return types.Position{ X: x, Y: y, Z: z}
}

func (c *Client) cleanupAvatars() {
   // Remove avatars that are no longer in the region (not seen for 2 minutes)
   for name, avatar := range c.status.NearbyAvatars {
      if time.Since(avatar.LastSeen) > 2*time.Minute {
         log.Printf("Avatar left region: %s last: %s", name, avatar.LastSeen.String())
         delete(c.status.NearbyAvatars, name)
      }
   }
}
