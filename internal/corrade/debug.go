package corrade

import (
   "net/url"
)

func formatURLValues(v url.Values) string {
   return v.Encode()
}
