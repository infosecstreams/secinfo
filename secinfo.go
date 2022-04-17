package main

import (
	"fmt"
	"os"

	"github.com/infosecstreams/secinfo/activity"
	"github.com/infosecstreams/secinfo/streamers"
)

func main() {
	f, err := streamers.OpenCSV("streamers.csv")
	if err != nil {
		os.Exit(1)
	}
	s, err := streamers.ParseStreamers(f)
	if err != nil {
		fmt.Println(err)
	}
	for _, streamer := range s.Streamers {
		fmt.Println(streamer.Name, streamer.YTURL)
	}
	activity.GetUID("jrozner")
}
