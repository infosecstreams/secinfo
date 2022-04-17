package streamers_test

import (
	"testing"

	"github.com/infosecstreams/secinfo/streamers"
	"github.com/spf13/afero"
)

func newTestSetup() afero.File {
	// Setup afero appfs memory fs and write test data to it
	appFS := afero.NewMemMapFs()
	a := afero.Afero{Fs: appFS}
	filename := "streamers.csv"
	afero.WriteFile(appFS, filename,
		[]byte("0reoByte,\n0xBufu,\n0xCardinal,\n0xChance,\n0xj3lly,\n0xRy4nG,https://www.youtube.com/channel/UCQWQlNq07_Rumy2i69dpqBw"), 0644)

	f, _ := a.Open(filename)
	return f
}

func TestParseStreamers(t *testing.T) {
	// Setup the test filesystem
	f := newTestSetup()

	// Parse the streamers.csv file
	sl, err := streamers.ParseStreamers(f)
	if err != nil {
		t.Errorf("Error: %s", err)
	}

	// Test if values in sl are equal to bytes written to streamers.csv
	if sl.Streamers[0].Name != "0reoByte" {
		t.Errorf("Got: %s, Wanted: %s", "0reoByte", sl.Streamers[0].Name)
	}

	// Test if the number of streamers in streamList is six
	if len(sl.Streamers) != 6 {
		t.Errorf("Got: %d, Wanted: %d", len(sl.Streamers), 6)
	}

	// Test is the sixth streamer has the correct YTURL
	if sl.Streamers[5].YTURL != "https://www.youtube.com/channel/UCQWQlNq07_Rumy2i69dpqBw" {
		t.Errorf("Got: %s, Wanted: %s", sl.Streamers[5].YTURL, "https://www.youtube.com/channel/UCQWQlNq07_Rumy2i69dpqBw")
	}

	// Test if the file fails to parse.
	f.Close()
	sl, err = streamers.ParseStreamers(f)
	if err != nil {
		t.Errorf("Error: %s", err)
	}
}
