package activity

import (
	"fmt"
	"net/http"
)

func GetUID(username string) (string, error) {
	// Make a net/http get request to get the UID
	// The URL is f'https://sullygnome.com/channel/%s/30/activitystats'
	r, err := http.Get("https://sullygnome.com/channel/" + username + "/30/activitystats")
	if err != nil {
		return "", err
	}
	// Print the response body
	fmt.Println(r.Body)

	return "", nil
}
