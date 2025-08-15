package slfunc

import (
   "strings"
)

// match Name Strings
func MatchName(a, b string) bool {
   return strings.EqualFold(NormalizeName(a), NormalizeName(b))
}

// remove Resident if it exists
func NormalizeName(name string) string {
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

func GetAvatarName(notification map[string]interface{}) string {
   if avi, ok := notification["name"]; ok {
      return avi.(string)
   } 
   
   return notification["firstname"].(string)+" "+notification["lastname"].(string) 
}  
