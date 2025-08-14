package corrade

import (
   "strings"
   "log"
   "errors"
)

func (c *Client) GoHome() error {
   params := map[string]string{
      "deanimate": "True",
      "fly": "False",
   }

   response, err := c.sendCommand("gohome", params)

   if err == nil && strings.Contains(response, "success") {
      log.Printf("heading home")
      return nil
   }
   return err
}

func (c *Client) IsOnline() bool {
   return c.status.IsOnline
}

func (c *Client) HomeRegion(home string) bool {
   thisSim := strings.TrimSpace(c.status.CurrentSim)
   homeSim := strings.TrimSpace(home)
   return strings.EqualFold(thisSim, homeSim)
}
   
func (c *Client) CheckRegion(home string) error {
   if !c.status.IsOnline {
      return errors.New("Bot Offline")
   }
      
   if !c.HomeRegion(home) {
      log.Printf("trying to return")
      return c.GoHome()
   }

   return nil
}
