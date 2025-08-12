package corrade

import (
   "slbot/internal/types"
   "errors"
   "strings"
)

func (c *Client) lookupByName(name string) (*types.AvatarInfo, error) {
   for _, v := range c.status.NearbyAvatars {
      if matchName(v.Name, name) {
         avatarInfoCopy := &types.AvatarInfo{
            Name:      v.Name,
            UUID:      v.UUID,
            Position:  v.Position,
            FirstSeen: v.FirstSeen,
            LastSeen:  v.LastSeen,
            IsGreeted: v.IsGreeted,
         }
         return avatarInfoCopy, nil
      }
   }
   return nil, errors.New("no Matching Avatar")
}

/*
// GetStatus returns the current bot status
func (c *Client) GetStatus() types.BotStatus {
   botInfo, _ := c.lookupByName(c.botName)
   return types.BotStatus {
      
   }
}
*/

// string match
func matchName(a, b string) bool {
   return strings.EqualFold(normalizeName(a), normalizeName(b))
}

// remove Resident if it exists
func normalizeName(name string) string {
   name = strings.TrimSpace(name)
   nameParts := strings.Split(name, " ")
   if len(nameParts) == 1 {
      return name
   }

   if strings.EqualFold(nameParts[1], "resident") {
      return nameParts[0]
   }

   return strings.Join(nameParts[:2], " ")
}
