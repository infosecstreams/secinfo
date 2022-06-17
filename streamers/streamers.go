/* Package streamers extracts 30-day streaming statistics and generates sorted markdown. */
// BUG(🐛): there are bugs in here.
package streamers

import (
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

// Streamer is a struct that contains the name of a streamer and the YouTube channel url.
// SullyGnomeID and ThirtyDayStats are fetched from SullyGnome.com.
// (sorry for lightly gathering a small amount of info every 24 hours).
type Streamer struct {
	Name           string  // The name of the streamer
	YTURL          string  // The url of the streamer's YouTube channel
	SullyGnomeID   string  // The SullyGnome ID of the streamer
	ThirtyDayStats float32 // Hours streamed in the last 30 days
	Lang           string  // The streamer's language. If they are online this is used in the generated markdown.
}

// StreamList is uhh... a list of Streamers.
type StreamerList struct {
	Streamers []Streamer // List of Streamers
}

// Len returns the length of the StreamerList, used to implement sort.Interface.
func (sl StreamerList) Len() int {
	return len(sl.Streamers)
}

// Less returns a bool for i > j, used to implment sort.Interface.
func (sl StreamerList) Less(i, j int) bool {
	return sl.Streamers[i].ThirtyDayStats > sl.Streamers[j].ThirtyDayStats
}

// Swap swaps the elements at i and j in the StreamerList, used to implement sort.Interface.
func (sl StreamerList) Swap(i, j int) {
	sl.Streamers[i], sl.Streamers[j] = sl.Streamers[j], sl.Streamers[i]
}

// Implement sort, after implementing the Len, Less, and Swap functions to satisfy the sort.Interface.
func (sl *StreamerList) Sort() {
	sort.Sort(sl)
}

// SullyGnomeStats is a struct to deserialize the 30-day streaming statistics json response.
type SullyGnomeStats struct {
	Data struct {
		Datasets []struct {
			Data []float32 `json:"data"`
		} `json:"datasets"`
	} `json:"data"`
}

// GetUID populates the Streamer struct's SullyGnomeID field.
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
		return
	}

	// Parse the body for '<span class="PageHeaderMiddleWithImageHeaderP1">'
	user_response := strings.Split(str_response, "<span class=\"PageHeaderMiddleWithImageHeaderP1\">")[1]
	// Remove everything after '</span>'
	user_response = strings.Split(user_response, "</span>")[0]
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
	s.Name = user_response
}

// GetStats populates the Streamer struct's ThirtyDayStats field with 30-day streaming statistics.
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

// OnlineNow returns a bool whether the streamer is online(🟢) or not in "index.md".
func (s *Streamer) OnlineNow(indexText string) bool {
	// Read index.md and search for the streamer's name to see if the line contains "🟢"
	for _, line := range strings.Split(indexText, "\n") {
		if strings.Contains(line, s.Name) {
			if strings.Contains(line, "🟢") {
				s.Lang = line[len(line)-2:]
				return true
			}
		}
	}
	return false
}

// ReturnMarkdownLine returns a GitHub markdown-flavored line for 'index.md' or 'inactive.md'.
// If the stream has > 0 hours over 30 days, it will return a line that contains columns for the 🟢 and Twitch/YouTube links.
// Otherwise if will return a line that just contains the streamer + links for inactive.md.
func (s Streamer) ReturnMarkdownLine(online bool) (string, error) {
	var line string
	if s.ThirtyDayStats > 0 { // active streamer
		if online { // online
			if s.YTURL != "" {
				line = fmt.Sprintf("🟢 | `%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s) &nbsp; [<i class=\"fab fa-youtube\" style=\"color:#C00\"></i>](%s) | %s\n", s.Name, s.Name, s.YTURL, s.Lang)
			} else {
				line = fmt.Sprintf("🟢 | `%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s) &nbsp; | %s\n", s.Name, s.Name, s.Lang)
			}
		} else { // offline
			if s.YTURL != "" {
				line = fmt.Sprintf("&nbsp; | `%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s) &nbsp; [<i class=\"fab fa-youtube\" style=\"color:#C00\"></i>](%s) |\n", s.Name, s.Name, s.YTURL)
			} else {
				line = fmt.Sprintf("&nbsp; | `%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s) &nbsp; |\n", s.Name, s.Name)
			}
		}
	} else { // inactive streamers
		if s.YTURL != "" {
			line = fmt.Sprintf("`%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s) &nbsp; [<i class=\"fab fa-youtube\" style=\"color:#C00\"></i>](%s)\n", s.Name, s.Name, s.YTURL)
		} else {
			line = fmt.Sprintf("`%s` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/%s) &nbsp;\n", s.Name, s.Name)
		}
	}
	return line, nil
}

// OpenCSV opens the CSV file and returns an Afero file object and/or error.
func OpenCSV(file string) (afero.File, error) {
	var AppFs = afero.NewOsFs()
	f, err := AppFs.Open(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return f, err
}

// ParseStreamers takes an Afero file object and returns a StreamerList populated with Streamer objects.
func ParseStreamers(f afero.File) (StreamerList, error) {
	// Test if the file exists and is not a directory
	i, _ := f.Stat()

	var sl StreamerList
	// Check if the file is a non-empty regular file
	if !i.IsDir() && i.Size() > 0 {
		// Read the file
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return sl, err
		}
		// Check the file to see if they are a CSV file
		for _, line := range strings.Split(string(b), "\n") {
			// Trim the line
			line = strings.TrimSpace(line)
			// Skip empty lines
			if line == "" {
				continue
			}
			if !strings.Contains(line, ",") {
				return sl, fmt.Errorf("file is not a CSV file: Text: %s", b)
			}
			parts := strings.Split(line, ",")
			// Append the streamer to the list
			sl.Streamers = append(sl.Streamers, Streamer{
				Name:  parts[0],
				YTURL: parts[1],
			})
		}
	} else {
		return sl, errors.New("file is not a file or is empty")
	}
	return sl, nil
}
