package model

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseManifest(t *testing.T) {
	xmlData := `
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" mediaPresentationDuration="PT5M59.7S">
  <Period>
    <AdaptationSet mimeType="video/mp4">
      <Representation id="1080p" bandwidth="4000000" width="1920" height="1080" />
    </AdaptationSet>
  </Period>
</MPD>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xmlData))
	}))
	defer ts.Close()

	mpd, err := ParseManifest(ts.URL)
	if err != nil {
		t.Fatalf("ParseManifest failed: %v", err)
	}

	if mpd.MediaPresentationDuration != "PT5M59.7S" {
		t.Errorf("expected duration PT5M59.7S, got %s", mpd.MediaPresentationDuration)
	}
}

func TestParseManifest_Fail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	_, err := ParseManifest(ts.URL)
	if err == nil {
		t.Error("expected error on 404, got nil")
	}
}

func TestSelectVideoRepresentation(t *testing.T) {
	mpd := &MPD{
		Period: Period{
			AdaptationSets: []AdaptationSet{
				{
					MimeType: "video/mp4",
					Representations: []Representation{
						{ID: "240p", Height: 240, Bandwidth: 200000},
						{ID: "360p", Height: 360, Bandwidth: 400000},
						{ID: "720p", Height: 720, Bandwidth: 1500000},
						{ID: "1080p", Height: 1080, Bandwidth: 4000000},
					},
				},
				{
					MimeType: "audio/mp4",
					Representations: []Representation{
						{ID: "audio", Bandwidth: 128000},
					},
				},
			},
		},
	}

	tests := []struct {
		name           string
		targetHeight   int
		expectedHeight int
		expectedID     string
	}{
		{"Exact Match 1080p", 1080, 1080, "1080p"},
		{"Exact Match 360p", 360, 360, "360p"},
		{"Fallback Lower (100p -> 240p)", 100, 240, "240p"},
		{"Fallback Higher (2000p -> 1080p)", 2000, 1080, "1080p"},
		{"Fallback Middle (500p -> 360p)", 500, 360, "360p"}, // |500-360|=140, |500-720|=220
		{"Fallback Middle (600p -> 720p)", 600, 720, "720p"}, // |600-360|=240, |600-720|=120
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep, err := mpd.SelectVideoRepresentation(tt.targetHeight)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rep.Height != tt.expectedHeight {
				t.Errorf("expected height %d, got %d", tt.expectedHeight, rep.Height)
			}
			if rep.ID != tt.expectedID {
				t.Errorf("expected ID %s, got %s", tt.expectedID, rep.ID)
			}
		})
	}
}

func TestSelectAudioRepresentation(t *testing.T) {
	mpd := &MPD{
		Period: Period{
			AdaptationSets: []AdaptationSet{
				{
					MimeType: "video/mp4",
					Representations: []Representation{
						{ID: "1080p", Height: 1080},
					},
				},
				{
					MimeType: "audio/mp4",
					Representations: []Representation{
						{ID: "audio1", Bandwidth: 128000},
						{ID: "audio2", Bandwidth: 64000},
					},
				},
			},
		},
	}

	rep, err := mpd.SelectAudioRepresentation()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep.ID != "audio1" { // Current implementation picks the first one
		t.Errorf("expected ID audio1, got %s", rep.ID)
	}
}

func TestParseManifest_XML(t *testing.T) {
	xmlData := `
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" mediaPresentationDuration="PT5M59.7S" minBufferTime="PT8S">
  <ProgramInformation>
    <Title>Test Video Title</Title>
  </ProgramInformation>
  <Period>
    <AdaptationSet mimeType="video/mp4">
      <Representation id="1080p" bandwidth="4000000" width="1920" height="1080">
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>
`
	var mpd MPD
	if err := xml.Unmarshal([]byte(xmlData), &mpd); err != nil {
		t.Fatalf("failed to unmarshal XML: %v", err)
	}

	if mpd.MediaPresentationDuration != "PT5M59.7S" {
		t.Errorf("expected duration PT5M59.7S, got %s", mpd.MediaPresentationDuration)
	}
	if mpd.ProgramInformation == nil || mpd.ProgramInformation.Title != "Test Video Title" {
		t.Errorf("expected title 'Test Video Title', got %v", mpd.ProgramInformation)
	}
}

func TestParseManifest_BadURL(t *testing.T) {
	// Test with a URL that causes http.Get to fail immediately (e.g. invalid protocol)
	_, err := ParseManifest("invalid-protocol://test")
	if err == nil {
		t.Error("expected error with invalid protocol, got nil")
	}
}

func TestSelectAudioRepresentation_None(t *testing.T) {
	mpd := &MPD{
		Period: Period{
			AdaptationSets: []AdaptationSet{
				{
					MimeType: "video/mp4",
					Representations: []Representation{
						{ID: "1080p", Height: 1080},
					},
				},
				// No audio adaptation set
			},
		},
	}

	_, err := mpd.SelectAudioRepresentation()
	if err == nil {
		t.Error("expected error when no audio representation present, got nil")
	}
}

func TestParseManifest_MalformedXML(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<MPD>this is not valid xml"))
	}))
	defer ts.Close()

	_, err := ParseManifest(ts.URL)
	if err == nil {
		t.Error("expected error on malformed XML, got nil")
	}
}

func TestSelectVideoRepresentation_None(t *testing.T) {
	mpd := &MPD{
		Period: Period{
			AdaptationSets: []AdaptationSet{
				{
					MimeType: "audio/mp4", // Only audio
					Representations: []Representation{
						{ID: "audio", Bandwidth: 100},
					},
				},
			},
		},
	}

	_, err := mpd.SelectVideoRepresentation(1080)
	if err == nil {
		t.Error("expected error when no video representation present, got nil")
	}
}
