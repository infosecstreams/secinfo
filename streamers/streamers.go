/* Package streamers loads StreamStatus's 30-day streaming statistics and generates sorted markdown. */
// BUG(🐛): there are bugs in here.
package streamers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/afero"
)

// Streamer is a struct that contains the name of a streamer and the YouTube channel url.
// ThirtyDayStats comes from StreamStatus's stats.json (see LoadBotStats).
type Streamer struct {
	Name           string  // The name of the streamer
	YTURL          string  // The url of the streamer's YouTube channel
	ThirtyDayStats float32 // Hours streamed in infosec categories in the last 30 days
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

// StreamStatus's stats API. StatsURL, HTTPClient, RetryDelays, Sleep and
// Now are variables so tests can point them at a fake server and clock.
var (
	StatsURL    = "https://p.infosecstreams.com/streamstatus/stats.json"
	HTTPClient  = &http.Client{Timeout: 30 * time.Second}
	RetryDelays = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second}
	Sleep       = time.Sleep
	Now         = time.Now
)

// ActiveMinHours is the roster rule: a streamer stays on (or comes back to)
// the main list with at least this many hours in infosec categories over
// the last 30 days, as StreamStatus recorded them. It is the smallest value
// the site shows, so nobody on the main list reads "0 h".
const ActiveMinHours float32 = 0.1

// MaxStatsAge is how old stats.json may be before it is too stale to move
// anyone (StreamStatus regenerates it every few minutes).
const MaxStatsAge = 6 * time.Hour

const userAgent = "secinfo (+https://github.com/infosecstreams/secinfo)"

// validLogin matches Twitch login names.
var validLogin = regexp.MustCompile(`^[A-Za-z0-9_]{1,25}$`)

// BotStreamer is one streamer in StreamStatus's stats.json. StreamStatus
// tracks streamers by Twitch user ID; Login is their current login.
type BotStreamer struct {
	ID          string  `json:"id"`
	Login       string  `json:"login"`
	DisplayName string  `json:"display_name"`
	Hours30d    float32 `json:"hours_30d"` // infosec categories only
}

type botStats struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Streamers   []BotStreamer `json:"streamers"`
}

// LoadBotStats fetches StreamStatus's stats.json and returns its streamers
// keyed by lower-case login. Anything that makes the data untrustworthy
// (outage, stale or empty file, malformed entries) is an error, so the
// caller writes nothing rather than moving streamers on bad data.
func LoadBotStats() (map[string]BotStreamer, error) {
	body, _, err := get(StatsURL)
	if err != nil {
		return nil, err
	}
	var st botStats
	if err := json.Unmarshal(body, &st); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", StatsURL, err)
	}
	if st.GeneratedAt.IsZero() {
		return nil, fmt.Errorf("%s has no generated_at", StatsURL)
	}
	if age := Now().Sub(st.GeneratedAt); age > MaxStatsAge {
		return nil, fmt.Errorf("%s is stale: generated %s ago", StatsURL, age.Round(time.Minute))
	}
	if len(st.Streamers) == 0 {
		return nil, fmt.Errorf("%s lists no streamers", StatsURL)
	}
	byLogin := make(map[string]BotStreamer, len(st.Streamers))
	for _, b := range st.Streamers {
		if !validLogin.MatchString(b.Login) || b.ID == "" || b.Hours30d < 0 {
			return nil, fmt.Errorf("%s has a malformed entry: %+v", StatsURL, b)
		}
		byLogin[strings.ToLower(b.Login)] = b
	}
	return byLogin, nil
}

// ApplyBotStats sets ThirtyDayStats from the streamer's stats.json entry
// and adopts its display-name capitalisation (never a different name). It
// reports whether StreamStatus knows the streamer at all.
func (s *Streamer) ApplyBotStats(stats map[string]BotStreamer) bool {
	b, ok := stats[strings.ToLower(s.Name)]
	if !ok {
		return false
	}
	s.ThirtyDayStats = b.Hours30d
	if strings.EqualFold(b.DisplayName, s.Name) {
		s.Name = b.DisplayName
	}
	return true
}

// Active reports whether the streamer meets the roster rule (see
// ActiveMinHours). Live and pre-live streamers are kept active separately.
func (s Streamer) Active() bool { return s.ThirtyDayStats >= ActiveMinHours }

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
