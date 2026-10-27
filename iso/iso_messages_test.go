package iso

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func loadTestSpec(t *testing.T) *IsoSpec {
	t.Helper()
	spec, err := LoadSpecsFromFile("../data/file_spec.json")
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func loadTemplateMessages(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("../data/template_messages.json")
	if err != nil {
		t.Fatal(err)
	}
	var templates []struct {
		Message         string `json:"message"`
		OriginalMessage string `json:"original_message"`
	}
	if err := json.Unmarshal(data, &templates); err != nil {
		t.Fatal(err)
	}
	var messages []string
	for _, tmpl := range templates {
		messages = append(messages, tmpl.Message)
		if tmpl.OriginalMessage != "" {
			messages = append(messages, tmpl.OriginalMessage)
		}
	}
	return messages
}

// Every bundled template must parse and format back to the same string.
func TestTemplatesRoundTrip(t *testing.T) {
	spec := loadTestSpec(t)
	for i, msg := range loadTemplateMessages(t) {
		isoMessage, err := NewIso8583Message(msg, spec)
		if err != nil {
			t.Errorf("template %d: parse error: %v", i, err)
			continue
		}
		if got := isoMessage.FormatIso(); got != msg {
			t.Errorf("template %d: round trip mismatch\n got: %s\nwant: %s", i, got, msg)
		}
	}
}

// Primary bitmap only (bit 1 clear): field 3 (processing code) and field 11 (STAN).
func TestPrimaryBitmapOnly(t *testing.T) {
	spec := loadTestSpec(t)
	msg := "0800" + "2020000000000000" + "990000" + "123456"
	isoMessage, err := NewIso8583Message(msg, spec)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got := isoMessage.GetField(10); got != "123456" {
		t.Errorf("STAN = %q, want 123456", got)
	}
	if got := isoMessage.FormatIso(); got != msg {
		t.Errorf("round trip mismatch\n got: %s\nwant: %s", got, msg)
	}
}

// Malformed input must return an error, never panic.
func TestMalformedMessages(t *testing.T) {
	spec := loadTestSpec(t)
	valid := loadTemplateMessages(t)[0]

	cases := map[string]string{
		"empty":                     "",
		"MTI only":                  "0100",
		"short primary bitmap":      "0100F23E",
		"non-hex primary bitmap":    "0100ZZZZZZZZZZZZZZZZ",
		"missing secondary bitmap":  "0100F23E440108E18062",
		"non-hex secondary bitmap":  "0100F23E440108E18062ZZZZZZZZZZZZZZZZ",
		"truncated fixed field":     "0800" + "2020000000000000" + "990000" + "123",
		"bad LLVar length":          "0100" + "4000000000000000" + "XX4242",
		"LLVar longer than message": "0100" + "4000000000000000" + "164242",
		"truncated template":        valid[:len(valid)/2],
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v", r)
				}
			}()
			if _, err := NewIso8583Message(msg, spec); err == nil {
				t.Errorf("expected an error for %q", msg)
			}
		})
	}
}

func TestFormatIsoAfterMarkField(t *testing.T) {
	spec := loadTestSpec(t)
	req, err := NewIso8583Message(loadTemplateMessages(t)[0], spec)
	if err != nil {
		t.Fatal(err)
	}
	resp := NewResponseMessage(req, "0110")
	resp.MarkField(38)
	resp.SetField(38, "00")

	parsed, err := NewIso8583Message(resp.FormatIso(), spec)
	if err != nil {
		t.Fatalf("response does not parse: %v", err)
	}
	if !strings.HasPrefix(resp.FormatIso(), "0110") || parsed.GetField(38) != "00" {
		t.Errorf("response MTI/field 39 not set: %s", resp.FormatIso())
	}
}
