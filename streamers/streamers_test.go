package streamers_test

import (
	"encoding/json"
	"fmt"
	"testing"

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
	if err.Error() != "file is not a CSV file: Text: text\ntext2" {
		t.Errorf("Got: %s, Wanted: %s", err, "file is not a CSV file: Text: text\ntext2")
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

func TestSullyGnomeStats(t *testing.T) {
	// Set test json
	j := "{\"data\": {\"datasets\": [{\"data\": [0,1,2,3,0,0,0,0,0]}]}}"

	// Parse the JSON into sullyGnomeStats object
	sg := streamers.SullyGnomeStats{}
	err := json.Unmarshal([]byte(j), &sg)
	if err != nil {
		t.Errorf("%s", err)
	}
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

func TestGetUserID(t *testing.T) {
	f, _ := AFS.Open("streamers.csv")

	// Parse the streamers.csv file
	sl, err := streamers.ParseStreamers(f)
	if err != nil {
		t.Errorf("%s", err)
	}
	for _, s := range sl.Streamers {
		s.GetUID()
	}
}

func TestGetStats(t *testing.T) {
	sl := streamers.StreamerList{
		Streamers: []streamers.Streamer{
			{Name: "fak3us3r", ThirtyDayStats: -1, SullyGnomeID: ""},
			{Name: "0xBufu", YTURL: "", SullyGnomeID: "36324233", ThirtyDayStats: 0},
			{Name: "0xCardinal", YTURL: "", SullyGnomeID: "41037834", ThirtyDayStats: 0},
			{Name: "0xChance", YTURL: "", SullyGnomeID: "5484638", ThirtyDayStats: 0},
			{Name: "0xRy4nG", YTURL: "https://www.youtube.com/channel/UCQWQlNq07_Rumy2i69dpqBw", SullyGnomeID: "6445036", ThirtyDayStats: 0},
		},
	}

	for _, s := range sl.Streamers {
		// s.GetUID()
		s.GetStats()
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
		s.GetUID()
		s.GetStats()
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
