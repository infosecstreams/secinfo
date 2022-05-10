package streamers

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/spf13/afero"
)

type Streamer struct {
	Name           string
	YTURL          string
	SullyGnomeID   string
	ThirtyDayStats float32
}

type StreamerList struct {
	Streamers []Streamer
}

func (sl StreamerList) Len() int {
	return len(sl.Streamers)
}

func (sl StreamerList) Less(i, j int) bool {
	return sl.Streamers[i].ThirtyDayStats > sl.Streamers[j].ThirtyDayStats
}

func (sl StreamerList) Swap(i, j int) {
	sl.Streamers[i], sl.Streamers[j] = sl.Streamers[j], sl.Streamers[i]
}

func (sl *StreamerList) Sort() {
	sort.Sort(sl)
}

type SullyGnomeStats struct {
	Data struct {
		Datasets []struct {
			Data []float32 `json:"data"`
		} `json:"datasets"`
	} `json:"data"`
}

func (s *Streamer) GetUID() {
	// Make a net/http get request to get the UID
	// The URL is f'https://sullygnome.com/channel/%s/30/activitystats'
	url := "https://sullygnome.com/channel/" + s.Name + "/30/activitystats"

	// Create a new GET request
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err)
	}

	// Set a User-Agent header
	request.Header.Set("user-agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:99.0) Gecko/20100101 Firefox/99.0")

	// Create a new http client
	client := &http.Client{}

	// Send the request
	r, err := client.Do(request)
	if err != nil {
		fmt.Println(err)
	}
	defer r.Body.Close()

	// Read the response
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
	}

	// Convert the response to a string
	str_response := string(b)

	// Check if response contains username
	if !strings.Contains(str_response, s.Name) {
		log.Printf("streamer hasn't streamed in a while! username not found, check spelling: %s, check twitch: https://www.twitch.tv/%s/schedule, stats: %s\n", s.Name, s.Name, url)
	}

	// Parse the body for 'var PageInfo = '
	str_response = strings.Split(str_response, "var PageInfo = ")[1]
	// Split on ;
	str_response = strings.Split(str_response, ";")[0]
	// Read the resulting string as json
	var j map[string]interface{}
	err = json.Unmarshal([]byte(str_response), &j)
	if err != nil {
		fmt.Println(err)
	}

	// Set the SullyGnomeID
	id := fmt.Sprintf("%.0f", j["id"])
	s.SullyGnomeID = id
}

func (s *Streamer) GetStats() {
	// Check that the streamer has a SullyGnomeID and not an empty string
	if s.SullyGnomeID == "" {
		fmt.Printf("streamer has no SullyGnomeID: %s\n", s.Name)
		return
	}

	// Make a new GET request to get the stats
	// The URL is f'https://sullygnome.com/api/charts/barcharts/getconfig/channelhourstreams/30/{uid}/{username}/%20/%20/0/0/%20/0/0/'
	request, err := http.NewRequest("GET", "https://sullygnome.com/api/charts/barcharts/getconfig/channelhourstreams/30/"+s.SullyGnomeID+"/"+s.Name+"/%20/%20/0/0/%20/0/0/", nil)
	if err != nil {
		fmt.Printf("Error creating request: %s\n", err)
	}

	// Set a User-Agent header
	request.Header.Set("user-agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:99.0) Gecko/20100101 Firefox/99.0")

	// Create a new http client
	client := &http.Client{}

	// Send the request
	r, err := client.Do(request)
	if err != nil {
		fmt.Printf("Error sending request: %s\n", err)
	}
	defer r.Body.Close()

	// Parse the JSON response into SullyGnomeStats struct
	var sg SullyGnomeStats
	err = json.NewDecoder(r.Body).Decode(&sg)
	if err != nil {
		fmt.Printf("Error decoding response: %s\n", err)
	}

	// Sum up the 30 day stats by mutiplying each data by index+1.0
	var sum float32
	for i, data := range sg.Data.Datasets[0].Data {
		sum += data * float32(i+1)
	}
	s.ThirtyDayStats = sum
}

func (s Streamer) OnlineNow(indexText string) bool {
	// Read index.md and search for the streamer's name to see if the line contains "🟢"
	for _, line := range strings.Split(indexText, "\n") {
		if strings.Contains(line, s.Name) {
			if strings.Contains(line, "🟢") {
				return true
			}
		}
	}
	return false
}

func (s Streamer) ReturnLine(online bool) (string, error) {
	var line string
	if s.ThirtyDayStats > 0 { // active streamer
		if online { // online
			if s.YTURL != "" {
				line = fmt.Sprintf("🟢 | `%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s) &nbsp; [<i class=\"fab a-youtube\" style=\"color:#C00\"></i>](%s) |\n", s.Name, s.Name, s.YTURL)
			} else {
				line = fmt.Sprintf("🟢 | `%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s) |\n", s.Name, s.Name)
			}
		} else { // offline
			if s.YTURL != "" {
				line = fmt.Sprintf("&nbsp; | `%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s) &nbsp; [<i class=\"fab fa-youtube\" style=\"color:#C00\"></i>](%s)\n", s.Name, s.Name, s.YTURL)
			} else {
				line = fmt.Sprintf("&nbsp; | `%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s)\n", s.Name, s.Name)
			}
		}
	} else { // inactive streamers
		if s.YTURL != "" {
			line = fmt.Sprintf("`%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s) &nbsp; [<i class=\"fab fa-youtube\" style=\"color:#C00\"></i>](%s)\n", s.Name, s.Name, s.YTURL)
		} else {
			line = fmt.Sprintf("`%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s)\n", s.Name, s.Name)
		}
	}
	return line, nil
}

func OpenCSV(file string) (afero.File, error) {
	var AppFs = afero.NewOsFs()
	f, err := AppFs.Open(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return f, err
}

func ParseStreamers(f afero.File) (StreamerList, error) {
	// Test if the file exists and is not a directory
	i, _ := f.Stat()

	// Check if the file is a non-empty regular file
	if !i.IsDir() && i.Size() > 0 {
		// Read the file
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return StreamerList{}, err
		}
		// Check the file to see if they are a CSV file
		for _, line := range strings.Split(string(b), "\n") {
			// Skip empty lines
			if line == "" {
				continue
			}
			if !strings.Contains(line, ",") {
				return StreamerList{}, fmt.Errorf("file is not a CSV file: Text: %s", b)
			}
			fmt.Printf("Line: %s\n", line)
		}
	} else {
		return StreamerList{}, errors.New("file is not a file or is empty")
	}

	// Open the file and parse the CSV
	c := csv.NewReader(f)
	r, err := c.ReadAll()
	if len(r) == 0 {
		return StreamerList{}, errors.New("file is empty")
	}
	if err != nil || len(r) == 0 {
		return StreamerList{}, err
	}
	// Create a slice of StreamerList
	var sl StreamerList
	// Print len of r
	fmt.Printf("Len: %d\n", len(r))
	// Fatally end program execution here
	log.Fatalf("%+v", r)

	for i, row := range r {
		// Announce we're here!
		fmt.Printf("Parsing: %s\n", row[i])
		// Skip empty lines
		if row[i] == "" {
			continue
		}
		sl.Streamers = append(sl.Streamers, Streamer{
			Name:  row[0],
			YTURL: row[1],
		})
	}
	return sl, nil
}
