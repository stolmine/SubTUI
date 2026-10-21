package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client for the self-hosted vector AutoDJ recommender (feishin-stolmine's
// autodj-server). POST /session/next takes seed and recent Navidrome track
// ids and returns the next track ids to queue.

type autoDJRequest struct {
	Count   int          `json:"count"`
	Exclude []string     `json:"exclude"`
	Params  autoDJParams `json:"params"`
	Recent  []string     `json:"recent"` // most recent last; drives the session centroid
	Seeds   []string     `json:"seeds"`
}

type autoDJParams struct {
	// 0 = maximum consistency, 1 = maximum variety; the server maps this one
	// value onto all of its selection knobs
	Contrast float64 `json:"contrast"`
}

type autoDJResponse struct {
	Tracks []string `json:"tracks"`
	Error  string   `json:"error"`
}

var autoDJClient = &http.Client{Timeout: 10 * time.Second}

// AutoDJNext asks the recommender for count track ids
func AutoDJNext(seeds, recent, exclude []string, count int, contrast float64) ([]string, error) {
	base := strings.TrimRight(strings.TrimSpace(AppConfig.AutoDJ.RecommenderURL), "/")
	if base == "" {
		return nil, fmt.Errorf("autodj: recommender_url not set")
	}

	body, err := json.Marshal(autoDJRequest{
		Count:   count,
		Exclude: nonNil(exclude),
		Params:  autoDJParams{Contrast: contrast},
		Recent:  nonNil(recent),
		Seeds:   nonNil(seeds),
	})
	if err != nil {
		return nil, err
	}

	resp, err := autoDJClient.Post(base+"/session/next", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("autodj: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out autoDJResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("autodj: bad response (%s): %w", resp.Status, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("autodj: %s: %s", resp.Status, out.Error)
	}

	return out.Tracks, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func SubsonicGetSong(id string) (Song, error) {
	data, err := subsonicGET("/getSong", url.Values{"id": {id}})
	if err != nil {
		return Song{}, err
	}

	return data.Response.Song, nil
}

// SubsonicGetSongs fetches songs by id concurrently, keeping their order and
// skipping any that fail
func SubsonicGetSongs(ids []string) []Song {
	songs := make([]Song, len(ids))
	ok := make([]bool, len(ids))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if s, err := SubsonicGetSong(id); err == nil && s.ID != "" {
				songs[i], ok[i] = s, true
			}
		}()
	}
	wg.Wait()

	out := make([]Song, 0, len(ids))
	for i := range songs {
		if ok[i] {
			out = append(out, songs[i])
		}
	}

	return out
}
