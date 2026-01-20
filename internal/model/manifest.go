package model

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

type MPD struct {
	XMLName                   xml.Name            `xml:"MPD"`
	MediaPresentationDuration string              `xml:"mediaPresentationDuration,attr"`
	MinBufferTime             string              `xml:"minBufferTime,attr"`
	ProgramInformation        *ProgramInformation `xml:"ProgramInformation"`
	Period                    Period              `xml:"Period"`
}

type ProgramInformation struct {
	Title string `xml:"Title"`
}

type Period struct {
	AdaptationSets []AdaptationSet `xml:"AdaptationSet"`
}

type AdaptationSet struct {
	ID              int              `xml:"id,attr"`
	MimeType        string           `xml:"mimeType,attr"`
	Representations []Representation `xml:"Representation"`
}

type Representation struct {
	ID              string          `xml:"id,attr"`
	Bandwidth       int             `xml:"bandwidth,attr"`
	Codecs          string          `xml:"codecs,attr"`
	Width           int             `xml:"width,attr"`
	Height          int             `xml:"height,attr"`
	SegmentTemplate SegmentTemplate `xml:"SegmentTemplate"`
}

type SegmentTemplate struct {
	Duration       int    `xml:"duration,attr"`
	Initialization string `xml:"initialization,attr"`
	Media          string `xml:"media,attr"`
	StartNumber    int    `xml:"startNumber,attr"`
	Timescale      int    `xml:"timescale,attr"`
}

func ParseManifest(url string) (*MPD, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch manifest, status: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest body: %w", err)
	}

	var mpd MPD
	if err := xml.Unmarshal(data, &mpd); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}

	return &mpd, nil
}

func (mpd *MPD) SelectVideoRepresentation(targetHeight int) (*Representation, error) {
	var bestRep *Representation
	var minDiff = 10000 // Arbitrary large number

	for _, as := range mpd.Period.AdaptationSets {
		if as.MimeType == "video/mp4" {
			for i := range as.Representations {
				rep := &as.Representations[i]
				diff := abs(rep.Height - targetHeight)
				if diff < minDiff {
					minDiff = diff
					bestRep = rep
				}
			}
		}
	}

	if bestRep == nil {
		return nil, fmt.Errorf("no video representation found")
	}

	return bestRep, nil
}

func (mpd *MPD) SelectAudioRepresentation() (*Representation, error) {
	for _, as := range mpd.Period.AdaptationSets {
		if as.MimeType == "audio/mp4" {
			if len(as.Representations) > 0 {
				return &as.Representations[0], nil
			}
		}
	}
	return nil, fmt.Errorf("no audio representation found")
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
