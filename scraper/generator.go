package scraper

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// --- 1. M3U Playlist Generator ---

// GenerateM3U builds the complete M3U playlist string
func GenerateM3U(channels []ChannelData, useGracenote bool, baseURL string) string {
	var sb strings.Builder

	// Provide the fallback XMLTV URL in the header for non-Gracenote channels or standard IPTV players
	sb.WriteString(fmt.Sprintf("#EXTM3U url-tvg=\"%s/epg.xml\"\n\n", baseURL))

	for _, ch := range channels {
		if ch.StreamURL == "" {
			continue
		}

		cleanName := strings.ReplaceAll(ch.Name, ",", "")
		cleanName = strings.ReplaceAll(cleanName, "\"", "'") // Protects the tvg-name attribute
		cleanGroup := strings.ReplaceAll(ch.Group, "\"", "'")

		// Standard EXTINF line
		sb.WriteString(fmt.Sprintf(
			`#EXTINF:-1 channel-id="tubi-%s" tvg-id="%s" tvg-name="%s" tvg-logo="%s" group-title="%s"`,
			ch.ID, ch.ID, cleanName, ch.Logo, cleanGroup,
		))

		// Inject Gracenote ID only if enabled and a valid mapping exists
		if useGracenote && ch.GracenoteID != "" {
			sb.WriteString(fmt.Sprintf(` tvc-guide-stationid="%s"`, ch.GracenoteID))
		}

		sb.WriteString(fmt.Sprintf(",%s\n%s\n\n", cleanName, ch.StreamURL))
	}

	return sb.String()
}

// --- 2. XMLTV Structs & Generator ---

type XMLTV struct {
	XMLName  xml.Name     `xml:"tv"`
	Channels []XMLChannel `xml:"channel"`
	Programs []XMLProgram `xml:"programme"`
}

type XMLChannel struct {
	ID          string   `xml:"id,attr"`
	DisplayName string   `xml:"display-name"`
	Icon        *XMLIcon `xml:"icon,omitempty"`
}

type XMLIcon struct {
	Src string `xml:"src,attr"`
}

type XMLProgram struct {
	Channel     string `xml:"channel,attr"`
	Start       string `xml:"start,attr"`
	Stop        string `xml:"stop,attr"`
	Title       string `xml:"title"`
	Description string `xml:"desc,omitempty"`
}

// GenerateXMLTV creates the XMLTV payload, skipping Gracenote channels when enabled
func GenerateXMLTV(channels []ChannelData, useGracenote bool) ([]byte, error) {
	tv := XMLTV{
		Channels: make([]XMLChannel, 0),
		Programs: make([]XMLProgram, 0),
	}

	for _, ch := range channels {
		// If Gracenote is enabled and this channel has an ID, skip it!
		// Channels DVR will fetch rich metadata directly, keeping our XML small and fast.
		if useGracenote && ch.GracenoteID != "" {
			continue
		}

		// 1. Add Channel Definition
		xmlChan := XMLChannel{
			ID:          ch.ID,
			DisplayName: ch.Name,
		}
		if ch.Logo != "" {
			xmlChan.Icon = &XMLIcon{Src: ch.Logo}
		}
		tv.Channels = append(tv.Channels, xmlChan)

		// 2. Add Broadcast Programmes
		for _, p := range ch.Programs {
			tv.Programs = append(tv.Programs, XMLProgram{
				Channel:     ch.ID,
				Start:       p.StartTime,
				Stop:        p.EndTime,
				Title:       p.Title,
				Description: p.Description,
			})
		}
	}

	xmlData, err := xml.MarshalIndent(tv, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal xmltv: %w", err)
	}

	header := []byte(xml.Header)
	return append(header, xmlData...), nil
}