package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	htmlTemplate "html/template"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/microcosm-cc/bluemonday"
	blackfriday "github.com/russross/blackfriday/v2"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/sooua/send.to/server/storage"
)

func (s *Server) previewHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Vary", "Range, Referer, X-Decrypt-Password")

	vars := mux.Vars(r)

	token := vars["token"]
	filename := vars["filename"]

	metadata, err := s.checkMetadata(r.Context(), token, filename)

	if err != nil {
		s.logger.Error("Error metadata", "error", err)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	contentType := metadata.ContentType

	// Preview templates (download.image.html, download.video.html, …) are not
	// shipped with this distribution — the Astro frontend handles the UI
	// instead. Bail out before doing any work: rendering a preview costs a
	// storage Head, a 5 MB read for text files and a QR encode, all of which
	// used to be thrown away by the redirect at the end of this handler.
	if !s.hasPreviewTemplates() {
		inlineURL, _ := url.Parse(path.Join(s.proxyPath, "inline", token, filename))
		http.Redirect(w, r, resolveURL(r, inlineURL, s.proxyPort), http.StatusFound)
		return
	}

	contentLength, err := s.storage.Head(r.Context(), token, filename)
	if err != nil {
		http.Error(w, http.StatusText(404), 404)
		return
	}

	var templatePath string
	var content htmlTemplate.HTML

	switch {
	case strings.HasPrefix(contentType, "image/"):
		templatePath = "download.image.html"
	case strings.HasPrefix(contentType, "video/"):
		templatePath = "download.video.html"
	case strings.HasPrefix(contentType, "audio/"):
		templatePath = "download.audio.html"
	case strings.HasPrefix(contentType, "text/"):
		templatePath = "download.markdown.html"

		var reader io.ReadCloser
		if reader, _, err = s.storage.Get(r.Context(), token, filename, nil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var data []byte
		data = make([]byte, _5M)
		if _, err = reader.Read(data); err != io.EOF && err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if strings.HasPrefix(contentType, "text/x-markdown") || strings.HasPrefix(contentType, "text/markdown") {
			unsafe := blackfriday.Run(data)
			output := bluemonday.UGCPolicy().SanitizeBytes(unsafe)
			content = htmlTemplate.HTML(output)
		} else if strings.HasPrefix(contentType, "text/plain") {
			content = htmlTemplate.HTML(fmt.Sprintf("<pre>%s</pre>", html.EscapeString(string(data))))
		} else {
			templatePath = "download.sandbox.html"
		}

	default:
		templatePath = "download.html"
	}

	relativeURL, _ := url.Parse(path.Join(s.proxyPath, token, filename))
	resolvedURL := resolveURL(r, relativeURL, s.proxyPort)
	relativeURLGet, _ := url.Parse(path.Join(s.proxyPath, getPathPart, token, filename))
	resolvedURLGet := resolveURL(r, relativeURLGet, s.proxyPort)
	var png []byte
	png, err = qrcode.Encode(resolvedURL, qrcode.High, 150)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	qrCode := base64.StdEncoding.EncodeToString(png)

	hostname := getURL(r, s.proxyPort).Host
	webAddress := resolveWebAddress(r, s.proxyPath, s.proxyPort)

	data := struct {
		ContentType    string
		Content        htmlTemplate.HTML
		Filename       string
		URL            string
		URLGet         string
		URLRandomToken string
		Hostname       string
		WebAddress     string
		ContentLength  uint64
		GAKey          string
		UserVoiceKey   string
		QRCode         string
	}{
		contentType,
		content,
		filename,
		resolvedURL,
		resolvedURLGet,
		token,
		hostname,
		webAddress,
		contentLength,
		s.gaKey,
		s.userVoiceKey,
		qrCode,
	}

	if htmlTemplates.Lookup(templatePath) == nil {
		inlineURL, _ := url.Parse(path.Join(s.proxyPath, "inline", token, filename))
		http.Redirect(w, r, resolveURL(r, inlineURL, s.proxyPort), http.StatusFound)
		return
	}

	if err := htmlTemplates.ExecuteTemplate(w, templatePath, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

}

// previewTemplates are every template previewHandler is able to render.
var previewTemplates = []string{
	"download.html",
	"download.image.html",
	"download.video.html",
	"download.audio.html",
	"download.markdown.html",
	"download.sandbox.html",
}

// hasPreviewTemplates reports whether any preview template was loaded from
// --web-path. Builds that ship only the Astro frontend have none.
func (s *Server) hasPreviewTemplates() bool {
	for _, name := range previewTemplates {
		if htmlTemplates.Lookup(name) != nil {
			return true
		}
	}
	return false
}

func (s *Server) headHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	token := vars["token"]
	filename := vars["filename"]

	metadata, err := s.checkMetadata(r.Context(), token, filename)

	if err != nil {
		s.logger.Error("Error metadata", "error", err)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	contentType := metadata.ContentType
	contentLength, err := s.storage.Head(r.Context(), token, filename)
	if s.storage.IsNotExist(err) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	} else if err != nil {
		s.logger.Error("Error", "error", err)
		http.Error(w, "Could not retrieve file.", http.StatusInternalServerError)
		return
	}

	remainingDownloads, remainingDays := metadata.remainingLimitHeaderValues()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatUint(contentLength, 10))
	w.Header().Set("Connection", "close")
	w.Header().Set("X-Remaining-Downloads", remainingDownloads)
	w.Header().Set("X-Remaining-Days", remainingDays)
	w.Header().Set("Vary", "Range, Referer, X-Decrypt-Password")

	if s.storage.IsRangeSupported() {
		w.Header().Set("Accept-Ranges", "bytes")
	}
}

func (s *Server) getHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	action := vars["action"]
	token := vars["token"]
	filename := vars["filename"]

	metadata, err := s.checkMetadata(r.Context(), token, filename)

	if err != nil {
		s.logger.Error("Error metadata", "error", err)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	var rng *storage.Range
	if r.Header.Get("Range") != "" {
		rng = storage.ParseRange(r.Header.Get("Range"))
	}

	contentType := metadata.ContentType
	reader, contentLength, err := s.storage.Get(r.Context(), token, filename, rng)
	defer storage.CloseCheck(reader)

	if s.storage.IsNotExist(err) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	} else if err != nil {
		s.metrics.downloadErrors.Add(1)
		s.logger.Error("Error", "error", err)
		http.Error(w, "Could not retrieve file.", http.StatusInternalServerError)
		return
	}
	if rng != nil {
		cr := rng.ContentRange()
		if cr != "" {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Range", cr)
			if rng.Limit > 0 {
				reader = io.NopCloser(io.LimitReader(reader, int64(rng.Limit)))
			}
		}
	}

	var disposition string
	if action == "inline" {
		disposition = "inline"
		/*
			metadata.ContentType is unable to determine the type of the content,
			So add text/plain in this case to fix XSS related issues/
		*/
		if strings.TrimSpace(contentType) == "" {
			contentType = "text/plain; charset=utf-8"
		}
	} else {
		disposition = "attachment"
	}

	remainingDownloads, remainingDays := metadata.remainingLimitHeaderValues()

	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Remaining-Downloads", remainingDownloads)
	w.Header().Set("X-Remaining-Days", remainingDays)

	password := r.Header.Get("X-Decrypt-Password")
	reader, err = attachDecryptionReader(reader, password)
	if err != nil {
		http.Error(w, "Could not decrypt file", http.StatusInternalServerError)
		return
	}

	if metadata.Encrypted && len(password) > 0 {
		contentType = metadata.DecryptedContentType
		contentLength = uint64(metadata.ContentLength)
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatUint(contentLength, 10))
	w.Header().Set("Vary", "Range, Referer, X-Decrypt-Password")

	if rng != nil && rng.ContentRange() != "" {
		w.WriteHeader(http.StatusPartialContent)
	}

	if disposition == "inline" && canContainsXSS(contentType) {
		reader = io.NopCloser(bluemonday.UGCPolicy().SanitizeReader(reader))
	}

	written, err := io.Copy(w, reader)
	if err != nil {
		s.metrics.downloadErrors.Add(1)
		s.logger.Error("Error", "error", err)
		http.Error(w, "Error occurred copying to output stream", http.StatusInternalServerError)
		return
	}

	s.metrics.downloads.Add(1)
	s.metrics.downloadBytes.Add(uint64(written))

	// A Range request is one slice of a resumed or seeking transfer, not a
	// distinct download. Counting them made `curl -C -` and every media
	// player that probes a file burn the Max-Downloads budget.
	if rng != nil {
		return
	}

	// The transfer already succeeded, so bookkeeping must not be skipped
	// just because the client hung up immediately afterwards.
	if err := s.increaseDownload(context.WithoutCancel(r.Context()), token, filename); err != nil {
		s.logger.Error("Could not record download", "token", token, "filename", filename, "error", err)
	}
}
