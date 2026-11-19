/* Package streamers extracts 30-day streaming statistics and generates sorted markdown. */
// BUG(🐛): there are bugs in here.
package streamers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

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
	WasInactive    bool    `json:"-"` // Whether the streamer came from inactive_streamers.csv
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

// SortByName sorts streamers by name case-insensitively for human-readable CSV output.
func (sl *StreamerList) SortByName() {
	sort.SliceStable(sl.Streamers, func(i, j int) bool {
		return strings.ToLower(sl.Streamers[i].Name) < strings.ToLower(sl.Streamers[j].Name)
	})
}

// ContainsStreamer returns true if a streamer with the same name exists in the list.
func (sl StreamerList) ContainsStreamer(streamer Streamer) bool {
	for _, s := range sl.Streamers {
		if strings.EqualFold(s.Name, streamer.Name) {
			return true
		}
	}
	return false
}

// RemoveStreamer returns a new list without the specified streamer (match by name).
func (sl StreamerList) RemoveStreamer(streamer Streamer) StreamerList {
	filtered := StreamerList{Streamers: make([]Streamer, 0, len(sl.Streamers))}
	for _, s := range sl.Streamers {
		if !strings.EqualFold(s.Name, streamer.Name) {
			filtered.Streamers = append(filtered.Streamers, s)
		}
	}
	return filtered
}

// SullyGnomeStats is a struct to deserialize the 30-day streaming statistics json response.
type SullyGnomeStats struct {
	Data struct {
		Datasets []struct {
			Data []float32 `json:"data"`
		} `json:"datasets"`
	} `json:"data"`
}

// SullyGnome access. BaseURL, HTTPClient, RetryDelays and Sleep are
// variables so tests can point them at a fake server.
var (
	BaseURL     = "https://sullygnome.com"
	HTTPClient  = &http.Client{Timeout: 30 * time.Second}
	RetryDelays = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second}
	Sleep       = time.Sleep
)

// ErrNotFound means SullyGnome has no channel page for the streamer, which
// happens when they haven't streamed for a long time (or the login is
// wrong). It is the only fetch failure that justifies demoting someone.
var ErrNotFound = errors.New("channel not found on SullyGnome")

const userAgent = "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:99.0) Gecko/20100101 Firefox/99.0"

// validLogin matches Twitch login names.
var validLogin = regexp.MustCompile(`^[A-Za-z0-9_]{1,25}$`)

// get fetches url, retrying transport errors, 429s and 5xx responses. It
// returns the body and the final URL after redirects.
func get(url string) ([]byte, string, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
		body, final, retry, err := getOnce(url)
		if err == nil {
			return body, final, nil
		}
		lastErr = err
		if !retry || attempt >= len(RetryDelays) {
			return nil, "", lastErr
		}
		Sleep(RetryDelays[attempt])
	}
}

func getOnce(url string) (body []byte, final string, retry bool, err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", false, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, "", true, err
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", true, err
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, "", true, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", false, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return body, resp.Request.URL.String(), false, nil
}

// pageInfo is the part of SullyGnome's `var PageInfo = {...}` we use.
type pageInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func parsePageInfo(html []byte) (pageInfo, error) {
	const marker = "var PageInfo = "
	i := bytes.Index(html, []byte(marker))
	if i < 0 {
		return pageInfo{}, errors.New("no PageInfo on channel page")
	}
	var pi pageInfo
	if err := json.NewDecoder(bytes.NewReader(html[i+len(marker):])).Decode(&pi); err != nil {
		return pageInfo{}, fmt.Errorf("decoding PageInfo: %w", err)
	}
	return pi, nil
}

// GetUID looks the streamer up on SullyGnome and sets SullyGnomeID. It
// returns ErrNotFound if SullyGnome has no page for them, and another error
// if the lookup itself failed (network, rate limiting, page format change),
// in which case nothing is known about the streamer.
func (s *Streamer) GetUID() error {
	if !validLogin.MatchString(s.Name) {
		return fmt.Errorf("%q is not a valid Twitch login", s.Name)
	}
	body, final, err := get(BaseURL + "/channel/" + s.Name + "/30/activitystats")
	if err != nil {
		return err
	}
	// Unknown channels redirect to the homepage.
	if !strings.Contains(final, "/channel/") {
		return ErrNotFound
	}
	pi, err := parsePageInfo(body)
	if err != nil {
		return fmt.Errorf("%s: %w", s.Name, err)
	}
	if pi.ID <= 0 {
		return ErrNotFound
	}
	s.SullyGnomeID = strconv.FormatInt(pi.ID, 10)
	// Adopt SullyGnome's capitalisation of the display name, but never a
	// different name (e.g. a localised display name).
	if strings.EqualFold(pi.Name, s.Name) {
		s.Name = pi.Name
	}
	return nil
}

// GetStats sets ThirtyDayStats from SullyGnome's 30-day hours chart. It
// needs SullyGnomeID (see GetUID).
func (s *Streamer) GetStats() error {
	if s.SullyGnomeID == "" {
		return fmt.Errorf("%s: no SullyGnomeID", s.Name)
	}
	body, _, err := get(BaseURL + "/api/charts/barcharts/getconfig/channelhourstreams/30/" + s.SullyGnomeID + "/" + s.Name + "/%20/%20/0/0/%20/0/0/")
	if err != nil {
		return err
	}
	var sg SullyGnomeStats
	if err := json.Unmarshal(body, &sg); err != nil {
		return fmt.Errorf("%s: decoding stats: %w", s.Name, err)
	}
	if len(sg.Data.Datasets) == 0 {
		return fmt.Errorf("%s: stats response has no datasets", s.Name)
	}
	// Weight each bucket by its index (bucket i holds streams of ~i+1 hours).
	var sum float32
	for i, data := range sg.Data.Datasets[0].Data {
		sum += data * float32(i+1)
	}
	s.ThirtyDayStats = sum
	return nil
}

// Row status markers StreamStatus writes in index.md's first column.
const (
	StatusOnline  = "🟢" // live now
	StatusPrelive = "🟡" // offline, but just switched to an infosec category
)

// IndexRow finds the streamer's row in index.md by their exact backticked
// name (so "Garr" doesn't match "Garr_7"), case-insensitively, and returns
// it with its status marker (StatusOnline, StatusPrelive or "").
func (s *Streamer) IndexRow(indexText string) (row, status string) {
	name := "`" + strings.ToLower(s.Name) + "`"
	for _, line := range strings.Split(indexText, "\n") {
		line = strings.TrimRight(line, " \r")
		if !strings.Contains(strings.ToLower(line), name) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		for _, st := range []string{StatusOnline, StatusPrelive} {
			if strings.HasPrefix(trimmed, st) {
				return line, st
			}
		}
		if row == "" {
			row = line
		}
	}
	return row, ""
}

// OnlineNow returns a bool whether the streamer is online(🟢) or not in "index.md".
func (s *Streamer) OnlineNow(indexText string) bool {
	line, status := s.IndexRow(indexText)
	if status != StatusOnline {
		return false
	}
	if len(line) >= 2 {
		s.Lang = line[len(line)-2:]
	}
	return true
}

// ReturnMarkdownLine returns a GitHub markdown-flavored line for 'index.md' or 'inactive.md'.
// If the stream has > 0 hours over 30 days or is online, it will return a line that contains columns for the 🟢 and Twitch/YouTube links.
// Otherwise if will return a line that just contains the streamer + links for inactive.md.
func (s Streamer) ReturnMarkdownLine(online bool) (string, error) {
	var line string
	// A live streamer is shown as active even with no 30-day stats yet.
	if s.ThirtyDayStats > 0 || online { // active streamer
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
	var sl StreamerList
	i, err := f.Stat()
	if err != nil {
		return sl, err
	}
	// Check if the file is a non-empty regular file
	if !i.IsDir() && i.Size() > 0 {
		// Read the file
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return sl, err
		}
		parsed, err := parseCSVData(string(b))
		if err != nil {
			return sl, err
		}
		sl = parsed
	} else {
		return sl, errors.New("file is not a file or is empty")
	}
	return sl, nil
}

// WriteCSV writes the streamer list to a CSV file sorted by name.
func (sl StreamerList) WriteCSV(filePath string) error {
	return sl.WriteCSVWithFS(afero.NewOsFs(), filePath)
}

// WriteCSVWithFS writes the streamer list to a CSV file sorted by name using the provided filesystem.
func (sl StreamerList) WriteCSVWithFS(fileSystem afero.Fs, filePath string) error {
	list := StreamerList{Streamers: append([]Streamer(nil), sl.Streamers...)}
	list.SortByName()

	content := buildCSVContent(list.Streamers)
	return afero.WriteFile(fileSystem, filePath, []byte(content), 0644)
}

// AppendToCSV adds a streamer to a CSV file and keeps it sorted by name.
func AppendToCSV(filePath string, streamer Streamer) error {
	return AppendToCSVWithFS(afero.NewOsFs(), filePath, streamer)
}

// AppendToCSVWithFS adds a streamer to a CSV file and keeps it sorted by name using the provided filesystem.
func AppendToCSVWithFS(fileSystem afero.Fs, filePath string, streamer Streamer) error {
	list, err := readCSVFile(fileSystem, filePath)
	if err != nil {
		return err
	}
	if list.ContainsStreamer(streamer) {
		return nil
	}
	list.Streamers = append(list.Streamers, streamer)
	return list.WriteCSVWithFS(fileSystem, filePath)
}

// RemoveFromCSV removes a streamer from a CSV file and keeps it sorted by name.
func RemoveFromCSV(filePath string, streamer Streamer) error {
	return RemoveFromCSVWithFS(afero.NewOsFs(), filePath, streamer)
}

// RemoveFromCSVWithFS removes a streamer from a CSV file and keeps it sorted by name using the provided filesystem.
func RemoveFromCSVWithFS(fileSystem afero.Fs, filePath string, streamer Streamer) error {
	list, err := readCSVFile(fileSystem, filePath)
	if err != nil {
		return err
	}
	list = list.RemoveStreamer(streamer)
	return list.WriteCSVWithFS(fileSystem, filePath)
}

func buildCSVContent(streamers []Streamer) string {
	var builder strings.Builder
	for _, s := range streamers {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(name)
		builder.WriteByte(',')
		builder.WriteString(strings.TrimSpace(s.YTURL))
	}
	return builder.String()
}

func readCSVFile(fileSystem afero.Fs, filePath string) (StreamerList, error) {
	list := StreamerList{}
	data, err := afero.ReadFile(fileSystem, filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return list, nil
		}
		return list, err
	}
	if len(data) == 0 {
		return list, nil
	}
	return parseCSVData(string(data))
}

// ParseCSV parses "name,youtube-url" lines. Any malformed line is an error.
func ParseCSV(data string) (StreamerList, error) { return parseCSVData(data) }

func parseCSVData(data string) (StreamerList, error) {
	list := StreamerList{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) < 2 {
			return StreamerList{}, fmt.Errorf("line %q has no comma (want name,youtube-url)", line)
		}
		name := strings.TrimSpace(parts[0])
		if !validLogin.MatchString(name) {
			return StreamerList{}, fmt.Errorf("line %q: %q is not a valid Twitch login", line, name)
		}
		list.Streamers = append(list.Streamers, Streamer{
			Name:  name,
			YTURL: strings.TrimSpace(parts[1]),
		})
	}
	return list, nil
}
