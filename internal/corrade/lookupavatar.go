package corrade

import (
   "slbot/internal/types"
   "slbot/internal/slfunc"
   "errors"
)

func (c *Client) lookupByName(name string) (*types.AvatarInfo, error) {
   for _, v := range c.status.NearbyAvatars {
      if slfunc.MatchName(v.Name, name) {
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

func (c *Client) GetBotName() string {
   return c.botName
}

func (c *Client) GetBotUUID() string {
   return c.botUUID
}
