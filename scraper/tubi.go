package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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

// Shared HTTP client with timeouts to avoid hanging sockets
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// Slugs to skip to avoid empty personalization arrays
var skipSlugs = map[string]bool{
	"favorite_linear_channels":    true,
	"recommended_linear_channels": true,
	"featured_channels":           true,
	"recently_added_channels":     true,
}

// Reconciled Gracenote Station IDs
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
	"613695":    "119661", // Fox Sports En Español
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

func FetchChannels() ([]ChannelData, error) {
	log.Println("[INFO] Fetching dynamic channel list from Tubi...")

	// 1. Fetch HTML
	req, _ := http.NewRequest("GET", "https://tubitv.com/live", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch live page: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	html := string(bodyBytes)

	// 2. Brace counting extraction
	marker := strings.Index(html, "window.__data")
	if marker == -1 {
		return nil, fmt.Errorf("window.__data marker not found")
	}

	openBrace := strings.Index(html[marker:], "{")
	if openBrace == -1 {
		return nil, fmt.Errorf("opening brace not found")
	}
	openBrace += marker

	depth := 0
	closeBrace := openBrace
	for i := openBrace; i < len(html); i++ {
		if html[i] == '{' {
			depth++
		} else if html[i] == '}' {
			depth--
			if depth == 0 {
				closeBrace = i
				break
			}
		}
	}

	blob := html[openBrace : closeBrace+1]

	// Clean JS constructs
	undefRe := regexp.MustCompile(`\bundefined\b`)
	blob = undefRe.ReplaceAllString(blob, "null")

	dateRe := regexp.MustCompile(`new\s+Date\("([^"]*)"\)`)
	blob = dateRe.ReplaceAllString(blob, `"$1"`)

	// 3. Parse JSON
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(blob), &data); err != nil {
		return nil, fmt.Errorf("JSON decode failed: %v", err)
	}

	epgObj, _ := data["epg"].(map[string]interface{})
	containers, _ := epgObj["contentIdsByContainer"].(map[string]interface{})

	var channelIDs []string
	groupMapping := make(map[string]string)
	seenIDs := make(map[string]bool)

	for _, containerListObj := range containers {
		containerList, ok := containerListObj.([]interface{})
		if !ok {
			continue
		}
		for _, catObjInterface := range containerList {
			cat, ok := catObjInterface.(map[string]interface{})
			if !ok {
				continue
			}

			slug, _ := cat["container_slug"].(string)
			if skipSlugs[slug] {
				continue
			}

			groupName := "Other"
			if name, ok := cat["name"].(string); ok && name != "" {
				groupName = name
			}

			if contents, ok := cat["contents"].([]interface{}); ok {
				for _, idObj := range contents {
					var idStr string
					switch v := idObj.(type) {
					case float64:
						idStr = fmt.Sprintf("%.0f", v)
					case string:
						idStr = v
					default:
						idStr = fmt.Sprintf("%v", v)
					}

					if !seenIDs[idStr] {
						seenIDs[idStr] = true
						channelIDs = append(channelIDs, idStr)
					}
					if _, exists := groupMapping[idStr]; !exists {
						groupMapping[idStr] = groupName
					}
				}
			}
		}
	}

	if len(channelIDs) == 0 {
		return nil, fmt.Errorf("0 channel IDs found after parsing")
	}
	log.Printf("[INFO] Extracted %d dynamic channels. Fetching manifests & EPG...", len(channelIDs))

	// 4. Batch fetch metadata and programs
	var finalChannels []ChannelData
	batchSize := 150

	for i := 0; i < len(channelIDs); i += batchSize {
		end := i + batchSize
		if end > len(channelIDs) {
			end = len(channelIDs)
		}
		batch := channelIDs[i:end]

		idStr := strings.Join(batch, ",")

		baseURL := "https://tubitv.com/oz/epg/programming"
		params := url.Values{}
		params.Add("content_id", idStr)

		epgURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

		epgResp, err := httpClient.Get(epgURL)
		if err != nil {
			log.Printf("[WARNING] Failed to fetch batch: %v", err)
			continue
		}

		var epgData struct {
			Rows []struct {
				ContentID      interface{} `json:"content_id"`
				Title          string      `json:"title"`
				CallSign       string      `json:"call_sign"`
				GracenoteID    string      `json:"gracenote_id"`
				VideoResources []struct {
					Manifest struct {
						URL string `json:"url"`
					} `json:"manifest"`
				} `json:"video_resources"`
				Images struct {
					Thumbnail []string `json:"thumbnail"`
				} `json:"images"`
				Programs []struct {
					Title       string `json:"title"`
					Description string `json:"description"`
					StartTime   string `json:"start_time"`
					EndTime     string `json:"end_time"`
				} `json:"programs"`
			} `json:"rows"`
		}

		if err := json.NewDecoder(epgResp.Body).Decode(&epgData); err != nil {
			log.Printf("[WARNING] Failed to decode batch: %v", err)
			epgResp.Body.Close()
			continue
		}
		epgResp.Body.Close()

		for _, row := range epgData.Rows {
			if len(row.VideoResources) == 0 {
				continue
			}

			var strID string
			switch v := row.ContentID.(type) {
			case float64:
				strID = fmt.Sprintf("%.0f", v)
			case string:
				strID = v
			default:
				strID = fmt.Sprintf("%v", v)
			}

			streamURL := CleanStreamURL(row.VideoResources[0].Manifest.URL)

			logo := ""
			if len(row.Images.Thumbnail) > 0 {
				logo = row.Images.Thumbnail[0]
			}

			// Parse program guide entries for fallback / non-Gracenote mode
			var parsedPrograms []ProgramData
			for _, p := range row.Programs {
				parsedPrograms = append(parsedPrograms, ProgramData{
					Title:       p.Title,
					Description: p.Description,
					StartTime:   formatTubiTime(p.StartTime),
					EndTime:     formatTubiTime(p.EndTime),
				})
			}

			// Prioritize our map; fall back to Tubi's API Gracenote ID if present
			gID := ManualGracenoteMap[strID]
			if gID == "" {
				gID = row.GracenoteID
			}

			finalChannels = append(finalChannels, ChannelData{
				ID:          strID,
				Name:        row.Title,
				CallSign:    row.CallSign,
				Group:       groupMapping[strID],
				Logo:        logo,
				StreamURL:   streamURL,
				GracenoteID: gID,
				Programs:    parsedPrograms,
			})
		}
	}

	return finalChannels, nil
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