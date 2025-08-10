package web

import (
   "log"
   "io/ioutil"
   "net/http"
   "bytes"
)

func printHTTPRequest( r *http.Request) {
	// Print the request method and URL
	log.Printf("Method: %s\n", r.Method)
	log.Printf("URL: %s\n", r.URL.String())

	// Print request headers
	log.Println("Headers:")
	for name, values := range r.Header {
		for _, value := range values {
			log.Printf("  %s: %s\n", name, value)
		}
	}

	// Print request body (if present)
	if r.Body != nil {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading request body: %v\n", err)
		} else {
			log.Printf("Body: %s\n", string(body))
		}
		// Important: Re-assign the body for subsequent handlers if needed
		r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	}
}
