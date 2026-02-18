package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/infosecstreams/secinfo/streamers"
	"github.com/spf13/afero"
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
		defer f.Close()

		activeFromFile, err := streamers.ParseStreamers(f)
		if err != nil {
			fmt.Println(err)
		}

		inactiveFromFile := streamers.StreamerList{}
		inactiveFile, err := streamers.OpenCSV("inactive_streamers.csv")
		if err == nil {
			defer inactiveFile.Close()
			inactiveFromFile, err = streamers.ParseStreamers(inactiveFile)
			if err != nil {
				fmt.Println(err)
			}
			for i := range inactiveFromFile.Streamers {
				inactiveFromFile.Streamers[i].WasInactive = true
			}
		} else if !os.IsNotExist(err) {
			fmt.Printf("Error reading inactive csv: %s\n", err)
		}

		// Only process active streamers from streamers.csv for stats
		// Inactive streamers are kept as-is without checking stats
		for _, streamer := range activeFromFile.Streamers {
			// Populate the streamer struct with the data SullyGnome has
			streamer.GetUID()
			if streamer.SullyGnomeID == "" {
				fmt.Printf("streamer has no SullyGnomeID: %s\n", streamer.Name)
				inactive.Streamers = append(inactive.Streamers, streamer)
				continue
			}
			streamer.GetStats()

			// Append the streamer to the new streamerList
			if streamer.ThirtyDayStats > 0 {
				active.Streamers = append(active.Streamers, streamer)
			} else {
				inactive.Streamers = append(inactive.Streamers, streamer)
			}
		}

		// Add all inactive streamers to the inactive list WITHOUT checking stats
		// (they remain inactive until manually moved back to streamers.csv)
		for _, streamer := range inactiveFromFile.Streamers {
			inactive.Streamers = append(inactive.Streamers, streamer)
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
	active.Sort()   // Sort active by ThirtyDayStats (descending)
	inactive.Sort() // Sort inactive by ThirtyDayStats for JSON

	// Write the active struct to active.json if SECINFO_TEST is not set so latest data is available
	if os.Getenv("SECINFO_TEST") == "" {
		appFS := afero.NewOsFs()
		j, _ := json.Marshal(active)
		ioutil.WriteFile("active.json", j, 0644)

		// write inactive.json
		j, _ = json.Marshal(inactive)
		ioutil.WriteFile("inactive.json", j, 0644)

		// Write updated CSV files, sorted by name for human readability
		activeCSVList := streamers.StreamerList{Streamers: active.Streamers}
		if err := activeCSVList.WriteCSVWithFS(appFS, "streamers.csv"); err != nil {
			fmt.Printf("Error writing streamers.csv: %s\n", err)
			os.Exit(1)
		}
		inactiveCSVList := streamers.StreamerList{Streamers: inactive.Streamers}
		if err := inactiveCSVList.WriteCSVWithFS(appFS, "inactive_streamers.csv"); err != nil {
			fmt.Printf("Error writing inactive_streamers.csv: %s\n", err)
			os.Exit(1)
		}
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

	// Sort inactive streamers by name for alphabetical display in markdown
	inactiveByName := streamers.StreamerList{Streamers: inactive.Streamers}
	inactiveByName.SortByName()

	for _, streamer := range inactiveByName.Streamers {
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
