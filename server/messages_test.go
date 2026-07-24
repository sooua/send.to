package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func TestNegotiateLang(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", "en"},
		{"en", "en"},
		{"zh", "zh"},
		{"zh-CN", "zh"},
		{"zh-Hans-SG,zh;q=0.9", "zh"},
		{"ja-JP", "ja"},
		{"*", "en"},
		{"de,fr", "en"},
		// Quality ordering decides, not the order they were written in.
		{"de;q=1.0,ja;q=0.8", "ja"},
		{"ja;q=0.2,zh;q=0.8", "zh"},
		// q=0 means "not acceptable", so it must not be selected.
		{"zh;q=0,ja", "ja"},
		{"nonsense", "en"},
	}

	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		if tc.header != "" {
			r.Header.Set("Accept-Language", tc.header)
		}

		if got := negotiateLang(r); got != tc.want {
			t.Errorf("negotiateLang(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}

func TestNegotiateLangWithoutRequest(t *testing.T) {
	if got := negotiateLang(nil); got != defaultLang {
		t.Errorf("negotiateLang(nil) = %q, want %q", got, defaultLang)
	}
}

func TestTranslateFallsBackToEnglish(t *testing.T) {
	if got := translate("de", msgNotFound); got != "Not Found" {
		t.Errorf("an unsupported language got %q, want the English text", got)
	}
	if got := translate("en", msgID("no-such-message")); got != "no-such-message" {
		t.Errorf("an unknown message rendered as %q; it must be visible, not an empty body", got)
	}
}

// verbs finds the format directives in a message, so a translation cannot
// quietly drop or reorder them. A mismatch here would ship
// "%!d(MISSING)" to a user in one language only.
var verbs = regexp.MustCompile(`%[-+ #0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z]`)

func TestEveryMessageIsTranslatedConsistently(t *testing.T) {
	languages := []string{"en", "zh", "ja"}

	for id, variants := range messages {
		english, ok := variants[defaultLang]
		if !ok {
			t.Errorf("%s has no English text, which is the fallback every other language relies on", id)
			continue
		}

		want := verbs.FindAllString(english, -1)

		for _, lang := range languages {
			text, ok := variants[lang]
			if !ok {
				t.Errorf("%s is missing a %s translation", id, lang)
				continue
			}

			got := verbs.FindAllString(text, -1)
			if len(got) != len(want) {
				t.Errorf("%s/%s has format verbs %v, English has %v", id, lang, got, want)
				continue
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("%s/%s format verb %d is %s, English has %s", id, lang, i, got[i], want[i])
				}
			}
		}
	}
}

func TestErrorBodyFollowsAcceptLanguage(t *testing.T) {
	srvr, _ := cachingTestServer(t)

	req := httptest.NewRequest("GET", "/nosuchtoken/a.txt", nil)
	req.Header.Set("Accept-Language", "ja-JP,ja;q=0.9")
	req = mux.SetURLVars(req, map[string]string{"token": "nosuchtoken", "filename": "a.txt"})

	w := httptest.NewRecorder()
	srvr.getHandler(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", w.Code)
	}
	if body := strings.TrimSpace(w.Body.String()); body != translate("ja", msgNotFound) {
		t.Errorf("body = %q, want the Japanese text", body)
	}
	if got := w.Header().Get("Content-Language"); got != "ja" {
		t.Errorf("Content-Language = %q, want ja", got)
	}
	if got := w.Header().Get("Vary"); !strings.Contains(got, "Accept-Language") {
		t.Errorf("Vary = %q, missing Accept-Language — a cache would serve one visitor's language to another", got)
	}
}

func TestErrorBodyStaysEnglishWithoutAcceptLanguage(t *testing.T) {
	srvr, _ := cachingTestServer(t)

	resp := download(srvr, "nosuchtoken", "a.txt", nil)

	if body := strings.TrimSpace(resp.Body.String()); body != "Not Found" {
		t.Errorf("body = %q, want the English text — existing clients must not change behaviour", body)
	}
}

func TestValidationErrorIsTranslated(t *testing.T) {
	srvr, _ := cachingTestServer(t)

	req := httptest.NewRequest("PUT", "/a.txt", strings.NewReader("hello"))
	req.ContentLength = 5
	req.Header.Set("Max-Days", "not-a-number")
	req.Header.Set("Accept-Language", "zh-CN")
	req = mux.SetURLVars(req, map[string]string{"filename": "a.txt"})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", w.Code)
	}
	if body := strings.TrimSpace(w.Body.String()); body != translate("zh", msgMaxDays) {
		t.Errorf("body = %q, want the Chinese text — validation errors reach the user too", body)
	}
}

func TestUserErrorKeepsAnEnglishErrorString(t *testing.T) {
	// The value still ends up in logs, where English is what an operator can
	// search for.
	err := userErrorf(msgMaxDaysTooLarge, 36500)

	if got, want := err.Error(), "Max-Days must be 36500 or less"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAddVaryKeepsExistingFields(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Vary", "Range, Referer")

	addVary(w, "Accept-Language")
	addVary(w, "accept-language")

	if got, want := w.Header().Get("Vary"), "Range, Referer, Accept-Language"; got != want {
		t.Errorf("Vary = %q, want %q", got, want)
	}
}
