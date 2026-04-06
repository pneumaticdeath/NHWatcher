package nao

// ServerConfig describes a dgamelaunch-based NetHack server.
type ServerConfig struct {
	Name       string // display name, e.g. "NAO", "Hardfought US"
	Key        string // short key for CLI, e.g. "nao", "hdf-us"
	SSHHost    string // host:port
	SSHUser    string
	TtyrecURL  string                     // base URL for ttyrec directory listings, empty to skip
	TtyrecPath func(player string) string // returns URL path from base to ttyrec dir
}

// Predefined servers.
var (
	ServerNAO = ServerConfig{
		Name:      "nethack.alt.org",
		Key:       "nao",
		SSHHost:   "nethack.alt.org:22",
		SSHUser:   "nethack",
		TtyrecURL: "https://alt.org/nethack/userdata",
		TtyrecPath: func(player string) string {
			return "/" + string(player[0]) + "/" + player + "/ttyrec/"
		},
	}

	// Hardfought servers do not have ttyrec fallback configured.
	// Their ttyrecs are archived to S3 (hdf-us/eu/au.s3.amazonaws.com)
	// and individual files are publicly downloadable but the buckets
	// are not listable. To add ttyrec support you would need to:
	//   1. POST to the PHP search endpoint to discover recordings:
	//      https://www.hardfought.org/nh/nethack/browsettyrec-{region}.php
	//      (POST body: player={name})
	//   2. Parse the HTML response for S3 download links
	//   3. Download and gzip-decompress the .ttyrec.gz files
	//   4. S3 URL format varies by region:
	//      US/EU: hdf-{region}.s3.amazonaws.com/ttyrec/{letter}/{player}/{variant}/{timestamp}.ttyrec.gz
	//      AU:    hdf-au.s3.amazonaws.com/ttyrec/{letter}/{player}/{variant}/ttyrec/{timestamp}.ttyrec.gz
	//   5. {variant} is the game variant directory (nethack, evilhack, etc.)

	ServerHardfoughtUS = ServerConfig{
		Name:    "us.hardfought.org",
		Key:     "hdf-us",
		SSHHost: "us.hardfought.org:22",
		SSHUser: "nethack",
	}

	ServerHardfoughtEU = ServerConfig{
		Name:    "eu.hardfought.org",
		Key:     "hdf-eu",
		SSHHost: "eu.hardfought.org:22",
		SSHUser: "nethack",
	}

	ServerHardfoughtAU = ServerConfig{
		Name:    "au.hardfought.org",
		Key:     "hdf-au",
		SSHHost: "au.hardfought.org:22",
		SSHUser: "nethack",
	}

	// AllServers is the default set of servers to cycle through.
	AllServers = []ServerConfig{
		ServerNAO,
		ServerHardfoughtUS,
		ServerHardfoughtEU,
		ServerHardfoughtAU,
	}

	// DefaultServers is just NAO for backward compatibility.
	DefaultServers = []ServerConfig{ServerNAO}
)

// ServersByKey returns the ServerConfigs matching the given keys.
// Unknown keys are silently ignored.
func ServersByKey(keys []string) []ServerConfig {
	all := map[string]ServerConfig{
		ServerNAO.Key:          ServerNAO,
		ServerHardfoughtUS.Key: ServerHardfoughtUS,
		ServerHardfoughtEU.Key: ServerHardfoughtEU,
		ServerHardfoughtAU.Key: ServerHardfoughtAU,
	}
	var result []ServerConfig
	for _, k := range keys {
		if s, ok := all[k]; ok {
			result = append(result, s)
		}
	}
	return result
}
