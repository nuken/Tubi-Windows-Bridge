package scraper

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ChannelData struct {
	ID          string
	Name        string
	CallSign    string
	Group       string
	Logo        string
	StreamURL   string
	GracenoteID string
	Programs    []ProgramData
}

type ProgramData struct {
	Title       string
	Description string
	StartTime   string
	EndTime     string
}

const (
	loginURL       = "https://account.production-public.tubi.io/user/login"
	channelsAPIURL = "https://tensor-cdn.production-public.tubi.io/api/v2/epg"
	epgAuthURL     = "https://epg-cdn.production-public.tubi.io/content/epg/programming"
	epgAnonURL     = "https://tubitv.com/oz/epg/programming"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

var skipSlugs = map[string]bool{
	"favorite_linear_channels":    true,
	"recommended_linear_channels": true,
	"featured_channels":           true,
	"recently_added_channels":     true,
}

var ManualGracenoteMap = map[string]string{
	"618762":    "113380", // ABC News Live
	"556174":    "114174", // NBC News NOW
	"555127":    "119219", // LiveNOW from FOX
	"618763":    "118952", // Localish
	"597678":    "114138", // Today All Day
	"555126":    "101103", // Cheddar
	"555129":    "116018", // theGrio
	"578086":    "120010", // Scripps News
	"560215":    "117478", // Estrella News
	"613761":    "121705", // NFL Channel
	"613765":    "62079",  // MLB
	"400000008": "133288", // NHL
	"400000116": "20923",  // The NBA Channel
	"400000294": "206205", // Watch AEW
	"400000286": "204956", // Scripps Sports Network
	"400000024": "144738", // DAZN Ringside
	"400000046": "176119", // PGA TOUR
	"557344":    "106100", // Fubo Sports Network
	"400000045": "120620", // PokerGO
	"400000012": "122277", // ACCDN
	"692073":    "124636", // Women's Sports Network
	"613758":    "120375", // beIN Sports XTRA
	"629323":    "129130", // NHRA TV
	"400000000": "121424", // Waypoint TV
	"613695":    "119661", // Fox Sports En Espa
	"400000247": "165474", // TV One Stars & Stories
	"692087":    "128385", // FilmRise Black TV
	"692262":    "121534", // Maverick Black Cinema
	"555382":    "119212", // FOX SOUL
	"684164":    "120372", // Bounce XL
	"400000056": "146144", // Ebony TV by Lionsgate
	"400000105": "118859", // Nash Bridges
	"684170":    "124040", // Grit Xtra
	"656574":    "122609", // The Carol Burnett Show
	"724210":    "128932", // The Rifleman
	"680705":    "122082", // Wanted: Dead or Alive
	"653199":    "120491", // Baywatch
	"400000066": "147893", // The FBI
	"400000289": "121204", // FailArmy
	"400000169": "115447", // Comedy Dynamics
	"653208":    "134109", // Always Funny
	"682059":    "125128", // Anger Management
	"682057":    "120237", // Are We There Yet
	"692114":    "116928", // LOL! Network
	"656575":    "113781", // Mystery Science Theater 3000
	"724209":    "121954", // Dr. G: Medical Examiner
	"400000249": "121659", // The Price Is Right: Barker
	"400000196": "125192", // Family Feud
	"653200":    "113452", // BUZZR
	"670602":    "122928", // Fear Factor USA
	"673411":    "116474", // Game Show Central
	"658748":    "121598", // Supermarket Sweep
	"677790":    "124358", // The Biggest Loser
	"670603":    "113735", // Wipeout Xtra
	"677011":    "114721", // Gordon Ramsay
	"715938":    "169611", // Sweet Escapes
	"692090":    "119052", // FilmRise Horror
	"692051":    "124998", // Horror by ALTER
	"400000033": "169820", // Kartoon Channel!
	"700414":    "128544", // Homeful
	"694174":    "128545", // Antiques Road Trip
	"700407":    "131923", // Garden with Monty Don
	"660350":    "114491", // The Bob Ross Channel
	"400000073": "169605", // In the Garage
	"715952":    "169315", // How To
	"715945":    "169645", // Welcome Home
	"400000065": "153566", // Top Gear
	"400000068": "170348", // Travel + Adventure
	"670587":    "115944", // MovieSphere
	"715950":    "170368", // At the Movies
	"673499":    "125052", // CINEVAULT
	"673500":    "125046", // CINEVAULT: Classics
	"673498":    "118942", // CINEVAULT: Westerns
	"400000069": "180081", // Classic Cinema
	"684167":    "123651", // ION Mystery
	"700415":    "122437", // Midsomer Murders
	"671073":    "118458", // Unsolved Mysteries
	"400000062": "123806", // BritBox Mysteries
	"400000195": "92255",  // Vice
	"400000048": "146284", // MrBeast
	"400000299": "123605", // Court TV
	"641492":    "119233", // Nosey
	"715947":    "169311", // Love & Marriage
	"692057":    "112881", // Alien Nation by DUST
	"700406":    "127985", // HauntTV
	"400000006": "135167", // OuterSphere
	"400000059": "120086", // Classic Doctor Who
	"700418":    "122114", // Love Nature
	"702891":    "132922", // The Jack Hanna Channel
	"702892":    "118923", // Xplore
	"400000063": "135387", // BBC Earth
	"400000251": "109553", // Law&Crime
	"400000096": "113780", // A&E Crime 360
	"400000094": "123431", // UnXplained Zone
	"400000095": "123430", // Crime Cults Killers
	"400000085": "129137", // Cold Case Files
	"400000091": "151234", // The FBI Files
	"692086":    "117358", // FilmRise True Crime
	"711410":    "113957", // Dateline 24/7
	"671083":    "149884", // Forensic Files
	"700412":    "131220", // True Crime Now
	"400000011": "145680", // TV One Crime & Justice
	"715949":    "169318", // Crime Scenes
	"400000074": "169596", // Chasing Criminals
	"400000067": "170382", // Living with Evil
	"682634":    "122912", // ION
	"684165":    "117518", // ION Plus
	"400000071": "169609", // Nikita
	"400000070": "169320", // Generation Drama
	"555124":    "146874", // Bloomberg TV+
	"571664":    "123870", // Bloomberg Originals
	"555130":    "124721", // CBC News
	"559144":    "121734", // Euronews
	"628893":    "121307", // FOX Weather
	"400000030": "132456", // Estrella Games
	"400000028": "132491", // Cine EstrellaTV
	"400000031": "117477", // EstrellaTV
	"555119":    "150390", // FOX LOCAL New York
	"557345":    "116946", // News 12 New York
	"555113":    "150392", // FOX LOCAL Los Angeles
	"555121":    "150388", // FOX LOCAL Washington DC
}

var (
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
	deviceID    string
)

func init() {
	deviceID = generateUUID()
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func getAuthToken(username, password string) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if cachedToken != "" && time.Now().Before(tokenExpiry.Add(-1*time.Minute)) {
		return cachedToken, nil
	}

	payload := map[string]interface{}{
		"type":      "email",
		"platform":  "web",
		"device_id": deviceID,
		"credentials": map[string]string{
			"email":    username,
			"password": password,
		},
		"errorLog": false,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to encode login payload: %w", err)
	}

	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", "https://tubitv.com")
	req.Header.Set("Referer", "https://tubitv.com/")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login rejected (status %d): %s", resp.StatusCode, string(respBody))
	}

	var authResp struct {
		AccessToken string  `json:"access_token"`
		ExpiresIn   float64 `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", fmt.Errorf("failed to decode login response: %w", err)
	}

	if authResp.AccessToken == "" {
		return "", fmt.Errorf("login succeeded but no access_token was returned")
	}

	ttl := authResp.ExpiresIn
	if ttl <= 0 {
		ttl = 3600
	}

	cachedToken = authResp.AccessToken
	tokenExpiry = time.Now().Add(time.Duration(ttl) * time.Second)
	log.Printf("[INFO] Authenticated as %s (token valid for %.0fs)", username, ttl)
	return cachedToken, nil
}

// FetchChannels queries Tubi's API using account credentials
func FetchChannels(username, password string) ([]ChannelData, error) {
	if username == "" || password == "" {
		return nil, fmt.Errorf("tubi credentials missing. Please set your Tubi email and password in settings")
	}

	token, err := getAuthToken(username, password)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	log.Println("[INFO] Fetching linear channel list via authenticated API...")

	req, err := http.NewRequest("GET", channelsAPIURL, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("mode", "tubitv_us_linear")
	q.Add("platform", "web")
	q.Add("device_id", deviceID)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("x-tubi-mode", "all")
	req.Header.Set("x-tubi-platform", "web")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("channels API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("channels API responded with HTTP %d", resp.StatusCode)
	}

	var epgResponse struct {
		Containers []struct {
			ContainerSlug string        `json:"container_slug"`
			Name          string        `json:"name"`
			Contents      []interface{} `json:"contents"`
		} `json:"containers"`
		Contents map[string]struct {
			NeedsLogin     bool   `json:"needs_login"`
			Title          string `json:"title"`
			CallSign       string `json:"call_sign"`
			GracenoteID    string `json:"gracenote_id"`
			VideoResources []struct {
				Manifest struct {
					URL string `json:"url"`
				} `json:"manifest"`
			} `json:"video_resources"`
			Images struct {
				Thumbnail []string `json:"thumbnail"`
			} `json:"images"`
		} `json:"contents"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&epgResponse); err != nil {
		return nil, fmt.Errorf("failed to decode channels response: %w", err)
	}

	// Map channel IDs to groups
	groupMapping := make(map[string]string)
	for _, c := range epgResponse.Containers {
		if skipSlugs[c.ContainerSlug] || c.Name == "" {
			continue
		}
		for _, rawID := range c.Contents {
			var idStr string
			switch v := rawID.(type) {
			case float64:
				idStr = fmt.Sprintf("%.0f", v)
			case string:
				idStr = v
			default:
				idStr = fmt.Sprintf("%v", v)
			}
			if _, exists := groupMapping[idStr]; !exists {
				groupMapping[idStr] = c.Name
			}
		}
	}

	var channelIDs []string
	channels := make([]ChannelData, 0, len(epgResponse.Contents))

	for id, item := range epgResponse.Contents {
		if item.NeedsLogin || len(item.VideoResources) == 0 {
			continue
		}

		manifestURL := item.VideoResources[0].Manifest.URL
		if manifestURL == "" {
			continue
		}

		logo := ""
		if len(item.Images.Thumbnail) > 0 {
			logo = item.Images.Thumbnail[0]
		}

		gID := ManualGracenoteMap[id]
		if gID == "" {
			gID = item.GracenoteID
		}

		group := groupMapping[id]
		if group == "" {
			group = "Other"
		}

		channels = append(channels, ChannelData{
			ID:          id,
			Name:        item.Title,
			CallSign:    item.CallSign,
			Group:       group,
			Logo:        logo,
			StreamURL:   CleanStreamURL(manifestURL),
			GracenoteID: gID,
		})
		channelIDs = append(channelIDs, id)
	}

	log.Printf("[INFO] Discovered %d live channels. Fetching program schedules...", len(channels))

	// Fetch EPG schedule in batches
	programsByChannel := fetchSchedules(channelIDs, token)
	for i := range channels {
		channels[i].Programs = programsByChannel[channels[i].ID]
	}

	return channels, nil
}

func fetchSchedules(channelIDs []string, token string) map[string][]ProgramData {
	programsMap := make(map[string][]ProgramData)
	batchSize := 150

	for i := 0; i < len(channelIDs); i += batchSize {
		end := i + batchSize
		if end > len(channelIDs) {
			end = len(channelIDs)
		}
		batch := channelIDs[i:end]
		idStr := strings.Join(batch, ",")

		req, err := http.NewRequest("GET", epgAuthURL, nil)
		if err != nil {
			continue
		}
		q := req.URL.Query()
		q.Add("platform", "web")
		q.Add("device_id", deviceID)
		q.Add("lookahead", "1")
		q.Add("content_id", idStr)
		req.URL.RawQuery = q.Encode()

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		req.Header.Set("x-tubi-mode", "all")
		req.Header.Set("x-tubi-platform", "web")
		req.Header.Set("User-Agent", userAgent)

		resp, err := httpClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			// Fall back to anonymous schedule endpoint
			if resp != nil {
				resp.Body.Close()
			}
			anonURL := fmt.Sprintf("%s?content_id=%s", epgAnonURL, url.QueryEscape(idStr))
			resp, err = httpClient.Get(anonURL)
			if err != nil || resp.StatusCode != http.StatusOK {
				if resp != nil {
					resp.Body.Close()
				}
				continue
			}
		}

		var epgData struct {
			Rows []struct {
				ContentID interface{} `json:"content_id"`
				Programs  []struct {
					Title       string `json:"title"`
					Description string `json:"description"`
					StartTime   string `json:"start_time"`
					EndTime     string `json:"end_time"`
				} `json:"programs"`
			} `json:"rows"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&epgData); err == nil {
			for _, row := range epgData.Rows {
				var strID string
				switch v := row.ContentID.(type) {
				case float64:
					strID = fmt.Sprintf("%.0f", v)
				case string:
					strID = v
				default:
					strID = fmt.Sprintf("%v", v)
				}

				var progs []ProgramData
				for _, p := range row.Programs {
					progs = append(progs, ProgramData{
						Title:       p.Title,
						Description: p.Description,
						StartTime:   formatTubiTime(p.StartTime),
						EndTime:     formatTubiTime(p.EndTime),
					})
				}
				programsMap[strID] = progs
			}
		}
		resp.Body.Close()
	}

	return programsMap
}

func CleanStreamURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.RawQuery = ""
	return parsed.String()
}

func formatTubiTime(timeStr string) string {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return timeStr
	}
	return t.Format("20060102150405 +0000")
}