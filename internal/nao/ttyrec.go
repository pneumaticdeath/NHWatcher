package nao

import (
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strings"

	"github.com/pneumaticdeath/NH_Watcher/internal/ttyrec"
)

const ttyrecBaseURL = "https://alt.org/nethack/userdata"

// ttyrecDirURL returns the URL for a player's ttyrec directory.
func ttyrecDirURL(player string) string {
	firstChar := string(player[0])
	return fmt.Sprintf("%s/%s/%s/ttyrec/", ttyrecBaseURL, firstChar, player)
}

// ttyrecFileRe matches ttyrec filenames in the directory listing.
var ttyrecFileRe = regexp.MustCompile(`href="(\d{4}-\d{2}-\d{2}\.\d{2}:\d{2}:\d{2}\.ttyrec)"`)

// listTTYRecs fetches the directory listing for a player and returns
// available ttyrec filenames.
func listTTYRecs(player string) ([]string, error) {
	resp, err := http.Get(ttyrecDirURL(player))
	if err != nil {
		return nil, fmt.Errorf("fetch dir listing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, player)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	matches := ttyrecFileRe.FindAllStringSubmatch(string(body), -1)
	var files []string
	for _, m := range matches {
		files = append(files, m[1])
	}
	return files, nil
}

// FetchRandomTTYRec tries to find and download a ttyrec recording from NAO.
// It checks the given player names (e.g. from the watch list) for available
// recordings and downloads a random one.
func FetchRandomTTYRec(players []string) ([]ttyrec.Frame, string, error) {
	// Shuffle players so we don't always check the same order
	shuffled := make([]string, len(players))
	copy(shuffled, players)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	// Collect all available recordings across players
	type recording struct {
		player string
		file   string
	}
	var available []recording

	for _, player := range shuffled {
		files, err := listTTYRecs(player)
		if err != nil {
			log.Printf("ttyrec listing for %s: %v", player, err)
			continue
		}
		for _, f := range files {
			available = append(available, recording{player, f})
		}
		// Stop searching once we have enough candidates
		if len(available) >= 10 {
			break
		}
	}

	if len(available) == 0 {
		return nil, "", fmt.Errorf("no ttyrec recordings found for any player")
	}

	// Pick a random recording
	chosen := available[rand.IntN(len(available))]
	url := fmt.Sprintf("%s%s", ttyrecDirURL(chosen.player), chosen.file)
	log.Printf("Downloading ttyrec: %s", url)

	frames, err := downloadTTYRec(url)
	if err != nil {
		return nil, "", err
	}

	label := fmt.Sprintf("%s (%s)", chosen.player,
		strings.TrimSuffix(chosen.file, ".ttyrec"))
	return frames, label, nil
}

// downloadTTYRec fetches and parses a ttyrec file from the given URL.
func downloadTTYRec(url string) ([]ttyrec.Frame, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	frames, err := ttyrec.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	if len(frames) == 0 {
		return nil, fmt.Errorf("empty ttyrec")
	}

	return frames, nil
}

// GetRecentPlayers connects to NAO and returns the list of players
// from the watch menu (for use as ttyrec candidates).
func (c *Client) GetRecentPlayers(cols, rows int) ([]string, error) {
	_, _, err := c.Connect(cols, rows)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if err := c.readUntilPrompt(); err != nil {
		return nil, err
	}
	if err := c.SendKey("w"); err != nil {
		return nil, err
	}

	menuOutput, err := c.readUntilWatchPrompt()
	if err != nil {
		return nil, err
	}

	games := ParseGameList(menuOutput)
	var players []string
	for _, g := range games {
		players = append(players, g.Player)
	}
	return players, nil
}
