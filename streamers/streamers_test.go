package streamers_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/infosecstreams/secinfo/streamers"
	"github.com/spf13/afero"
)

var (
	// Setup afero appfs memory fs and write test data to it
	FS  afero.Fs     = afero.NewMemMapFs()
	AFS *afero.Afero = &afero.Afero{Fs: FS}
)

func init() {
	afero.WriteFile(AFS, "streamers.csv",
		[]byte("fakeus3r,\n0reoByte,\n \n0xBufu,\n0xCardinal,\n0xChance,\n0xj3lly,\n0xRy4nG,https://www.youtube.com/channel/UCQWQlNq07_Rumy2i69dpqBw\nantisyphon,https://www.youtube.com/channel/UCkFKiCm7dD0gsB4jqIdCuRQ\nSecurity_Live,https://www.youtube.com/channel/UCMDy1HAPNcpl8zVTK1NfMqw"), 0644)
	afero.WriteFile(AFS, "index.md",
		[]byte("---: | --- | :--- | :---\n🟢 | `Security_Live` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/Security_Live) &nbsp; [<i class=\"fab a-youtube\" style=\"color:#C00\"></i>](https://www.youtube.com/channel/UCMDy1HAPNcpl8zVTK1NfMqw) |\n&nbsp; | `S4vitaar` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/S4vitaar) &nbsp; [<i class=\"fab fa-youtube\" style=\"color:#C00\"></i>](https://www.youtube.com/channel/UCNHWpNqiM8yOQcHXtsluD7Q)\n🟢 | `jbeers11` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/jbeers11) |\n&nbsp; | `SecurityWeekly` | [<i class=\"fab fa-twitch\" style=\"color:#9146FF\"></i>](https://www.twitch.tv/SecurityWeekly) &nbsp; [<i class=\"fab fa-youtube\" style=\"color:#C00\"></i>](https://www.youtube.com/channel/UCg--XBjJ50a9tUhTKXVPiqg)\n"), 0644)
	afero.WriteFile(AFS, "streamers2.csv", []byte(""), 0644)
	afero.WriteFile(AFS, "not_a_csv.csv", []byte("text\ntext2"), 0644)
}

func TestParseStreamersPass(t *testing.T) {
	f, _ := AFS.Open("streamers.csv")

	// Parse the streamers.csv file
	sl, err := streamers.ParseStreamers(f)
	if err != nil {
		t.Fatalf("Error: %s", err)
	}

	// Test if values in sl are equal to bytes written to streamers.csv
	if sl.Streamers[1].Name != "0reoByte" {
		t.Errorf("Got: %s, Wanted: %s", "0reoByte", sl.Streamers[1].Name)
	}

	// Test if the number of streamers in streamList is six
	if len(sl.Streamers) != 9 {
		t.Errorf("Got: %d, Wanted: %d", len(sl.Streamers), 9)
	}

	// Test is the sixth streamer has the correct YTURL
	if sl.Streamers[6].YTURL != "https://www.youtube.com/channel/UCQWQlNq07_Rumy2i69dpqBw" {
		t.Errorf("Got: %s, Wanted: %s", sl.Streamers[6].YTURL, "https://www.youtube.com/channel/UCQWQlNq07_Rumy2i69dpqBw")
	}
}

func TestParseStreamersFail(t *testing.T) {
	if e, err := AFS.Exists("doesnt_exist.csv"); err == nil {
		t.Logf("File doesn't exist: %t", e)
	}
	d, err := AFS.ReadDir("/")
	if err != nil {
		t.Errorf("%s", err)
	}
	for _, f := range d {
		fmt.Println(f.Mode(), f.ModTime(), f.Size(), f.IsDir(), f.Name())
	}

	f, _ := AFS.Open("streamers2.csv")
	s, err := f.Stat()
	if err != nil {
		t.Errorf("%s", err)
	}
	if s.Size() == 0 {
		t.Logf("File is empty")
	}

	// Parse the streamers.csv file
	sl, err := streamers.ParseStreamers(f)
	if err == nil {
		t.Fatal("Error: test expected to fail!")
	}
	if sl.Len() != 0 {
		t.Errorf("Got: %d, Wanted: %d", sl.Len(), 0)
	}

	f, _ = AFS.Open("not_a_csv.csv")
	s, err = f.Stat()
	if err != nil {
		t.Errorf("%s", err)
	}
	if s.Size() == 0 {
		t.Logf("File is empty, as expected: %d", s.Size())
	}

	// Parse the streamers.csv file
	sl, err = streamers.ParseStreamers(f)
	if err == nil {
		t.Fatal("Error: test expected to fail!")
	}
	if want := `line "text" has no comma (want name,youtube-url)`; err.Error() != want {
		t.Errorf("Got: %s, Wanted: %s", err, want)
	}
	t.Logf("Received error as expected: %s", err)
	sl = streamers.StreamerList{Streamers: []streamers.Streamer{}}
}

func TestSort(t *testing.T) {
	sl := streamers.StreamerList{
		Streamers: []streamers.Streamer{
			{Name: "0reoByte", ThirtyDayStats: 1},
			{Name: "0xBufu", ThirtyDayStats: 3},
			{Name: "0xCardinal", ThirtyDayStats: 4},
			{Name: "Security_Live", ThirtyDayStats: 137},
		},
	}

	// Sort the streamer list
	sl.Sort()
}

func TestOpenCSVPass(t *testing.T) {
	f, err := AFS.Open("streamers.csv")
	if err != nil {
		t.Errorf("%s", err)
	}
	if f == nil {
		t.Errorf("File is nil")
	}
}

func TestOpenCSVFail(t *testing.T) {
	f, err := streamers.OpenCSV("streamers.csv")
	if err == nil {
		t.Errorf("%s", err)
	}
	if f != nil {
		t.Errorf("File is not nil")
	}
}

func TestOnlineNow(t *testing.T) {
	f, _ := AFS.Open("streamers.csv")
	idx, _ := AFS.Open("index.md")

	// Read idx into a string
	idxBytes, _ := afero.ReadAll(idx)
	idxStr := string(idxBytes)

	// Parse the streamers.csv file
	sl, err := streamers.ParseStreamers(f)
	if err != nil {
		t.Errorf("Error: %s", err)
	}
	for _, s := range sl.Streamers {
		if s.OnlineNow(idxStr) {
			fmt.Printf("%s is 🟢online now\n", s.Name)
		} else {
			fmt.Printf("%s is OFFLINE now\n", s.Name)
		}
	}
}

func TestReturnMarkdownLine(t *testing.T) {
	f, _ := AFS.Open("streamers.csv")

	// Parse the streamers.csv file
	sl, err := streamers.ParseStreamers(f)
	if err != nil {
		t.Errorf("%s", err)
	}
	for _, s := range sl.Streamers {
		if s.YTURL != "" && s.Name == "0xRy4nG" {
			s.ThirtyDayStats = 0
			s.ReturnMarkdownLine(true)
			s.ReturnMarkdownLine(false)
		} else if s.YTURL != "" && s.Name == "Security_Live" {
			s.ThirtyDayStats = 20
			s.ReturnMarkdownLine(true)
			s.ReturnMarkdownLine(false)
		}
		s.ReturnMarkdownLine(true)
		s.ReturnMarkdownLine(false)
		fmt.Printf("Object: %+v", s)
	}
}

func TestWriteCSVSortsByName(t *testing.T) {
	list := streamers.StreamerList{Streamers: []streamers.Streamer{
		{Name: "bob", YTURL: ""},
		{Name: "Alice", YTURL: ""},
		{Name: "charlie", YTURL: ""},
	}}

	dir := t.TempDir()
	filePath := filepath.Join(dir, "streamers.csv")

	if err := list.WriteCSV(filePath); err != nil {
		t.Fatalf("WriteCSV failed: %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	got := string(data)
	want := "Alice,\nbob,\ncharlie,"
	if got != want {
		t.Fatalf("Got: %q, Wanted: %q", got, want)
	}
	if strings.HasSuffix(got, "\n") {
		t.Fatalf("CSV should not end with a newline")
	}
}

func TestAppendRemoveCSV(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "streamers.csv")
	if err := os.WriteFile(filePath, []byte("bob,\ncharlie,"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := streamers.AppendToCSV(filePath, streamers.Streamer{Name: "Alice"}); err != nil {
		t.Fatalf("AppendToCSV failed: %v", err)
	}
	data, _ := os.ReadFile(filePath)
	got := string(data)
	want := "Alice,\nbob,\ncharlie,"
	if got != want {
		t.Fatalf("Got: %q, Wanted: %q", got, want)
	}

	if err := streamers.RemoveFromCSV(filePath, streamers.Streamer{Name: "bob"}); err != nil {
		t.Fatalf("RemoveFromCSV failed: %v", err)
	}
	data, _ = os.ReadFile(filePath)
	got = string(data)
	want = "Alice,\ncharlie,"
	if got != want {
		t.Fatalf("Got: %q, Wanted: %q", got, want)
	}
}

func TestContainsRemoveStreamer(t *testing.T) {
	list := streamers.StreamerList{Streamers: []streamers.Streamer{
		{Name: "Alice"},
		{Name: "Bob"},
	}}
	if !list.ContainsStreamer(streamers.Streamer{Name: "alice"}) {
		t.Fatalf("ContainsStreamer should match case-insensitively")
	}
	filtered := list.RemoveStreamer(streamers.Streamer{Name: "bob"})
	if filtered.ContainsStreamer(streamers.Streamer{Name: "bob"}) {
		t.Fatalf("RemoveStreamer should remove by name")
	}
}

func TestOnlineNowMatchesExactName(t *testing.T) {
	index := strings.Join([]string{
		"---: | --- | :--- | :---",
		"🟢 | `Garr_7` | [x](https://www.twitch.tv/Garr_7 \"Just Chatting\") &nbsp; | EN",
		"&nbsp; | `Garr` | [x](https://www.twitch.tv/Garr) &nbsp; |",
		"🟢 | `TheLaluka` | [x](https://www.twitch.tv/TheLaluka) &nbsp; | FR",
	}, "\n")
	tests := []struct {
		name     string
		want     bool
		wantLang string
	}{
		{"Garr_7", true, "EN"},
		{"Garr", false, ""}, // a substring of Garr_7 must not count
		{"thelaluka", true, "FR"},
		{"nobody", false, ""},
	}
	for _, tt := range tests {
		s := streamers.Streamer{Name: tt.name}
		if got := s.OnlineNow(index); got != tt.want || s.Lang != tt.wantLang {
			t.Errorf("%s: OnlineNow = %v (lang %q), want %v (lang %q)", tt.name, got, s.Lang, tt.want, tt.wantLang)
		}
	}
}

// A live streamer with no 30-day stats yet must render as an active, online
// row, not an inactive.md row.
func TestReturnMarkdownLineOnlineWithoutStats(t *testing.T) {
	s := streamers.Streamer{Name: "Garr_7", YTURL: "https://www.youtube.com/c/x", Lang: "EN"}
	got, _ := s.ReturnMarkdownLine(true)
	if !strings.HasPrefix(got, "🟢 | `Garr_7` |") || !strings.HasSuffix(got, "| EN\n") {
		t.Errorf("got %q, want an online active row", got)
	}
	got, _ = s.ReturnMarkdownLine(false)
	if !strings.HasPrefix(got, "`Garr_7` |") {
		t.Errorf("offline with no stats: got %q, want an inactive row", got)
	}
}

func TestIndexRowStatus(t *testing.T) {
	index := strings.Join([]string{
		"🟢 | `live` | x | EN",
		"🟡 | `soon` | x | DE",
		"&nbsp; | `idle` | x |",
	}, "\n")
	for name, want := range map[string]string{"live": streamers.StatusOnline, "SOON": streamers.StatusPrelive, "idle": "", "nobody": ""} {
		s := streamers.Streamer{Name: name}
		if _, got := s.IndexRow(index); got != want {
			t.Errorf("%s: status %q, want %q", name, got, want)
		}
	}
	soon := streamers.Streamer{Name: "soon"}
	if soon.OnlineNow(index) {
		t.Error("a 🟡 row is not online")
	}
}

func TestParseCSVRejectsBadLines(t *testing.T) {
	for _, in := range []string{"alice,\nbob\n", "al ice,\n", "ali\tce,x\n"} {
		if _, err := streamers.ParseCSV(in); err == nil {
			t.Errorf("ParseCSV(%q) succeeded, want an error", in)
		}
	}
	got, err := streamers.ParseCSV(" alice , https://yt \n\nbob,\n")
	if err != nil || len(got.Streamers) != 2 || got.Streamers[0].Name != "alice" || got.Streamers[0].YTURL != "https://yt" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func fakeStats(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	oldURL, oldSleep, oldNow := streamers.StatsURL, streamers.Sleep, streamers.Now
	streamers.StatsURL, streamers.Sleep = srv.URL, func(time.Duration) {}
	streamers.Now = func() time.Time { return time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { streamers.StatsURL, streamers.Sleep, streamers.Now = oldURL, oldSleep, oldNow })
}

const goodStats = `{"generated_at":"2026-11-21T09:55:04Z","streamers":[
 {"id":"1191005071","login":"censoredhacker","display_name":"CensoredHacker","hours_30d":10.4,"streams_30d":7},
 {"id":"54550064","login":"0reobyte","display_name":"0reoByte","hours_30d":0}]}`

func TestLoadBotStats(t *testing.T) {
	fakeStats(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(goodStats)) })
	stats, err := streamers.LoadBotStats()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 || stats["censoredhacker"].Hours30d != 10.4 || stats["0reobyte"].ID != "54550064" {
		t.Errorf("got %+v", stats)
	}

	s := streamers.Streamer{Name: "censoredHacker"}
	if !s.ApplyBotStats(stats) || s.ThirtyDayStats != 10.4 || s.Name != "CensoredHacker" || !s.Active() {
		t.Errorf("ApplyBotStats: %+v", s)
	}
	o := streamers.Streamer{Name: "0reoByte", ThirtyDayStats: 99}
	if !o.ApplyBotStats(stats) || o.ThirtyDayStats != 0 || o.Active() {
		t.Errorf("0 hours must replace old stats and be inactive: %+v", o)
	}
	if n := (streamers.Streamer{Name: "nobody"}); n.ApplyBotStats(stats) {
		t.Error("unknown login reported as known")
	}
}

func TestLoadBotStatsRetriesThenSucceeds(t *testing.T) {
	calls := 0
	fakeStats(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable) // status.json is 503 until the first reconcile
			return
		}
		w.Write([]byte(goodStats))
	})
	if _, err := streamers.LoadBotStats(); err != nil || calls != 3 {
		t.Errorf("err=%v after %d calls, want success on the 3rd", err, calls)
	}
}

// Anything that makes the data untrustworthy is an error, never "0 hours".
func TestLoadBotStatsRejectsBadData(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"outage":    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) },
		"not found": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) },
		"html":      func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>Just a moment...</html>")) },
		"no date": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"streamers":[{"id":"1","login":"a"}]}`))
		},
		"stale": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"generated_at":"2026-11-21T03:00:00Z","streamers":[{"id":"1","login":"a"}]}`))
		},
		"empty": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"generated_at":"2026-11-21T09:55:04Z","streamers":[]}`))
		},
		"bad login": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"generated_at":"2026-11-21T09:55:04Z","streamers":[{"id":"1","login":"a b"}]}`))
		},
		"no id": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"generated_at":"2026-11-21T09:55:04Z","streamers":[{"login":"a"}]}`))
		},
		"negative h": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"generated_at":"2026-11-21T09:55:04Z","streamers":[{"id":"1","login":"a","hours_30d":-1}]}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			fakeStats(t, h)
			if stats, err := streamers.LoadBotStats(); err == nil {
				t.Errorf("got %+v, want an error", stats)
			}
		})
	}
}
