package streamers

import (
	"encoding/csv"
	"errors"
	"io/fs"

	"github.com/spf13/afero"
)

type streamer struct {
	Name  string
	YTURL string
}

type streamerList struct {
	Streamers []streamer
}

func OpenCSV(file string) (afero.File, error) {
	var AppFs = afero.NewOsFs()
	f, err := AppFs.Open(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return f, nil
}

func ParseStreamers(f afero.File) (streamerList, error) {
	// Open the file and parse the CSV
	c := csv.NewReader(f)
	r, err := c.ReadAll()
	if err != nil {
		return streamerList{}, err
	}
	// Create a slice of streamerList
	var sl streamerList
	for _, row := range r {
		sl.Streamers = append(sl.Streamers, streamer{
			Name:  row[0],
			YTURL: row[1],
		})
	}
	return sl, nil
}
