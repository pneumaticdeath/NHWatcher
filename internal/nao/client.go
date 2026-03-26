// Package nao handles connections to nethack.alt.org for spectating games.
package nao

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	sshHost = "nethack.alt.org:22"
	sshUser = "nethack"
)

// selectorChars matches dgamelaunch's selector sequence (no 'q' or 'Q').
const selectorChars = "abcdefghijklmnoprstuvwxyzABCDEFGHIJKLMNOPRSTUVWXYZ"

// Game represents a game listed in the dgamelaunch watch menu.
type Game struct {
	Selector string // e.g. "a", "b", ...
	Player   string
	Cols     int
	Rows     int
	Idle     string // blank means actively playing
}

// FitsIn returns true if the game's terminal fits within the given dimensions.
func (g Game) FitsIn(cols, rows int) bool {
	return g.Cols <= cols && g.Rows <= rows
}

// IsIdle returns true if the game has been idle (> 4 seconds per dgamelaunch).
func (g Game) IsIdle() bool {
	return g.Idle != ""
}

// Client manages an SSH connection to nethack.alt.org for spectating.
type Client struct {
	mu      sync.Mutex
	session *ssh.Session
	client  *ssh.Client
	stdin   io.WriteCloser
	stdout  io.Reader
	ptyCols int
	ptyRows int
}

// NewClient creates a new NAO client.
func NewClient() *Client {
	return &Client{}
}

// Connect establishes an SSH connection to NAO and returns
// reader/writer for the terminal session. The cols and rows parameters
// set the PTY size, which should match the terminal widget dimensions.
func (c *Client) Connect(cols, rows int) (io.WriteCloser, io.Reader, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ptyCols = cols
	c.ptyRows = rows

	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(""),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	var err error
	c.client, err = ssh.Dial("tcp", sshHost, config)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh dial: %w", err)
	}

	c.session, err = c.client.NewSession()
	if err != nil {
		c.client.Close()
		return nil, nil, fmt.Errorf("ssh session: %w", err)
	}

	// Identify ourselves to dgamelaunch
	c.session.Setenv("DGAME_CLIENT_TYPE", "nhwatcher")

	if err := c.session.RequestPty("xterm-256color", rows, cols, ssh.TerminalModes{
		ssh.ECHO: 0,
	}); err != nil {
		c.session.Close()
		c.client.Close()
		return nil, nil, fmt.Errorf("request pty: %w", err)
	}

	c.stdin, err = c.session.StdinPipe()
	if err != nil {
		c.session.Close()
		c.client.Close()
		return nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}

	c.stdout, err = c.session.StdoutPipe()
	if err != nil {
		c.session.Close()
		c.client.Close()
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := c.session.Shell(); err != nil {
		c.session.Close()
		c.client.Close()
		return nil, nil, fmt.Errorf("start shell: %w", err)
	}

	return c.stdin, c.stdout, nil
}

// SendKey sends a single keystroke to the remote session.
func (c *Client) SendKey(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return fmt.Errorf("not connected")
	}
	_, err := c.stdin.Write([]byte(key))
	return err
}

// WatchRandomGame navigates the dgamelaunch menus to spectate a random
// non-idle game. The avoid parameter specifies a player name to skip
// (e.g. the previously watched player). Pass "" to skip nobody.
func (c *Client) WatchRandomGame(avoid string) (string, error) {
	// Wait for the initial dgamelaunch menu to appear
	if err := c.readUntilPrompt(); err != nil {
		return "", fmt.Errorf("waiting for main menu: %w", err)
	}

	// Send 'w' to enter watch mode
	if err := c.SendKey("w"); err != nil {
		return "", fmt.Errorf("send watch key: %w", err)
	}

	// Read the watch menu and parse the game list
	menuOutput, err := c.readUntilWatchPrompt()
	if err != nil {
		return "", fmt.Errorf("reading watch menu: %w", err)
	}

	games := ParseGameList(menuOutput)
	if len(games) == 0 {
		clean := stripANSI(menuOutput)
		log.Printf("Watch menu raw output (%d bytes):\n%s", len(menuOutput), clean)
		return "", fmt.Errorf("no games in progress")
	}

	// Filter to games that fit our PTY and are not idle,
	// avoiding the previously watched player if possible.
	var active []Game
	var fitting []Game
	for _, g := range games {
		if !g.FitsIn(c.ptyCols, c.ptyRows) {
			continue
		}
		fitting = append(fitting, g)
		if !g.IsIdle() {
			active = append(active, g)
		}
	}

	// Prefer non-idle games that fit, then any that fit, then any game
	candidates := active
	if len(candidates) == 0 {
		candidates = fitting
	}
	if len(candidates) == 0 {
		candidates = games
	}

	// Try to avoid the previously watched player
	if avoid != "" && len(candidates) > 1 {
		var filtered []Game
		for _, g := range candidates {
			if g.Player != avoid {
				filtered = append(filtered, g)
			}
		}
		if len(filtered) > 0 {
			candidates = filtered
		}
	}

	// Pick one at random
	chosen := candidates[rand.IntN(len(candidates))]

	// Send the selector key to start watching
	if err := c.SendKey(chosen.Selector); err != nil {
		return "", fmt.Errorf("send selector: %w", err)
	}

	return chosen.Player, nil
}

// readUntilPrompt reads from stdout until we see the dgamelaunch main menu prompt.
func (c *Client) readUntilPrompt() error {
	return c.readUntil("=>")
}

// readUntilWatchPrompt reads from stdout until we see the watch menu prompt,
// returning all accumulated output for parsing.
func (c *Client) readUntilWatchPrompt() (string, error) {
	return c.readUntilCapture("=>")
}

// readUntil reads from stdout until the given marker appears, discarding output.
func (c *Client) readUntil(marker string) error {
	_, err := c.readUntilCapture(marker)
	return err
}

// readUntilCapture reads from stdout until marker appears, returning all output.
func (c *Client) readUntilCapture(marker string) (string, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 1024)
	for {
		n, err := c.stdout.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			if strings.Contains(buf.String(), marker) {
				return buf.String(), nil
			}
		}
		if err != nil {
			return buf.String(), fmt.Errorf("read: %w", err)
		}
	}
}

// Close shuts down the current SSH connection. The client can be
// reconnected by calling Connect again.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		c.session.Close()
		c.session = nil
	}
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
	c.stdin = nil
	c.stdout = nil
}

// stripANSI removes ANSI escape sequences from terminal output.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\([0-9A-B]`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

// ParseGameList extracts games from dgamelaunch watch menu output.
// After ANSI stripping, entries run together (no newlines) with a watcher
// count between them. Example:
//   a) Badger004        nh367  182x 35  2026-03-23 16:46:50  12m 24s  0b) BatBeefs ...
// Idle column is blank for actively playing games (idle <= 4s).
func ParseGameList(output string) []Game {
	clean := stripANSI(output)
	var games []Game

	// Match each game entry anywhere in the text.
	// Captures: 1=selector, 2=player, 3=cols, 4=rows, 5=idle time (may be empty)
	gameRe := regexp.MustCompile(`([a-pr-zA-PR-Z])\)\s+(\S+)\s+\S+\s+(\d+)x\s*(\d+)\s+\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\s+((?:\d+[hms]\s*(?:\d+[hms]\s*)?)?)\s*\d`)

	for _, m := range gameRe.FindAllStringSubmatch(clean, -1) {
		cols, _ := strconv.Atoi(m[3])
		rows, _ := strconv.Atoi(m[4])
		idle := strings.TrimSpace(m[5])
		games = append(games, Game{
			Selector: m[1],
			Player:   m[2],
			Cols:     cols,
			Rows:     rows,
			Idle:     idle,
		})
	}
	return games
}
