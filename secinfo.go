package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/infosecstreams/secinfo/streamers"
)

func main() {
	active := streamers.StreamerList{}
	inactive := streamers.StreamerList{}

	// Check environ SECINFO_TEST exists
	if os.Getenv("SECINFO_TEST") == "" {
		f, err := streamers.OpenCSV("streamers.csv")
		if err != nil {
			fmt.Printf("Error reading csv: %s\n", err)
			os.Exit(1)
		}

		s, err := streamers.ParseStreamers(f)
		if err != nil {
			fmt.Println(err)
		}

		for _, streamer := range s.Streamers {
			// Populate the streamer struct with the data SullyGnome has
			streamer.GetUID()
			streamer.GetStats()

			// Append the streamer to the new streamerList
			if streamer.ThirtyDayStats > 0 {
				active.Streamers = append(active.Streamers, streamer)
			} else {
				inactive.Streamers = append(inactive.Streamers, streamer)
			}
		}
	} else {
		// Read test.json into active struct
		f, _ := ioutil.ReadFile("active.json")
		_ = json.Unmarshal([]byte(f), &active)

		// Read inactive.json into inactive struct
		f, _ = ioutil.ReadFile("inactive.json")
		_ = json.Unmarshal([]byte(f), &inactive)
	}

	// Call sort on the active streamer list
	active.Sort()
	inactive.Sort()

	// Write the active struct to active.json if SECINFO_TEST is not set so latest data is available
	if os.Getenv("SECINFO_TEST") == "" {
		j, _ := json.Marshal(active)
		ioutil.WriteFile("active.json", j, 0644)

		// write inactive.json
		j, _ = json.Marshal(inactive)
		ioutil.WriteFile("inactive.json", j, 0644)
	}

	// Markdown time!
	// Read existing index.md into a string
	indexMd, _ := ioutil.ReadFile("index.md")
	indexStr := string(indexMd)

	// Read index.tmpl.md into a string
	indexMdTemplate, _ := ioutil.ReadFile("templates/index.tmpl.md")
	// Find  '---: | --- | :--- | :---' and append each streamer in streamerist using ReturnMarkdownLine()
	heading := "---: | --- | :--- | :---\n"
	i := strings.Index(string(indexMdTemplate), heading) + len(heading)
	// Print line from the i indexMD
	newMd := string(indexMdTemplate[:i])
	for _, streamer := range active.Streamers {
		s, err := streamer.ReturnMarkdownLine(streamer.OnlineNow(indexStr))
		if err != nil {
			fmt.Println(err)
		}
		newMd += s
	}
	newMd += string(indexMdTemplate[i:])
	// Write index.md
	ioutil.WriteFile("./index.md", []byte(newMd), 0644)

	// Clear markdown string
	newMd = ""
	// Read inactive.tmpl.md into a string
	inactiveMD, _ := ioutil.ReadFile("templates/inactive.tmpl.md")
	// Fine '--: | --- | :--- | :---' and append each streamer in inactive using ReturnMarkdownLine()
	heading = "--: | ---\n"
	i = strings.Index(string(inactiveMD), heading) + len(heading)
	// Print line to the i indexMD
	newMd = string(inactiveMD[:i])
	for _, streamer := range inactive.Streamers {
		s, err := streamer.ReturnMarkdownLine(false) // Sorry inactive can't be online
		if err != nil {
			fmt.Println(err)
		}
		newMd += s
	}
	// Print line from the i indexMD
	newMd += string(inactiveMD[i:])
	// Write inactive.md
	ioutil.WriteFile("./inactive.md", []byte(newMd), 0644)
}
