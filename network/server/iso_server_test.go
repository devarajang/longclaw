package network

import "testing"

// A response must match only the request it answers: same reference and same message class/function.
func TestMatchKey(t *testing.T) {
	for _, c := range []struct {
		request, response string
		match             bool
	}{
		{"0100", "0110", true},
		{"0120", "0130", true},
		{"0200", "0210", true},
		{"0220", "0230", true},
		{"0420", "0430", true},
		{"0100", "0430", false}, // a reversal's response must not match the original authorisation
		{"0200", "0230", false}, // an advice's response must not match the original purchase
		{"0100", "0210", false},
	} {
		got := matchKey(c.request, "000000000001") == matchKey(c.response, "000000000001")
		if got != c.match {
			t.Errorf("request %s / response %s: match = %v, want %v", c.request, c.response, got, c.match)
		}
	}
	if matchKey("0100", "000000000001") == matchKey("0110", "000000000002") {
		t.Error("different references must not match")
	}
}
