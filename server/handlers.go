/*
The MIT License (MIT)

Copyright (c) 2025 sooua

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	htmlTemplate "html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	textTemplate "text/template"
	"time"
	"unicode"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/ProtonMail/gopenpgp/v2/constants"
	"github.com/sooua/send.to/server/storage"

	"github.com/gorilla/mux"
	"golang.org/x/net/idna"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const getPathPart = "get"

var (
	htmlTemplates = initHTMLTemplates()
	textTemplates = initTextTemplates()
)

func stripPrefix(path string) string {
	return strings.Replace(path, "/", "", 1)
}

func initTextTemplates() *textTemplate.Template {
	templateMap := textTemplate.FuncMap{"format": formatNumber}

	// Templates with functions available to them
	var templates = textTemplate.New("").Funcs(templateMap)
	return templates
}

func initHTMLTemplates() *htmlTemplate.Template {
	templateMap := htmlTemplate.FuncMap{"format": formatNumber}

	// Templates with functions available to them
	var templates = htmlTemplate.New("").Funcs(templateMap)

	return templates
}

func attachEncryptionReader(reader io.ReadCloser, password string) (io.ReadCloser, error) {
	if len(password) == 0 {
		return reader, nil
	}

	return encrypt(reader, []byte(password))
}

func attachDecryptionReader(reader io.ReadCloser, password string) (io.ReadCloser, error) {
	if len(password) == 0 {
		return reader, nil
	}

	return decrypt(reader, []byte(password))
}

func decrypt(ciphertext io.ReadCloser, password []byte) (plaintext io.ReadCloser, err error) {
	unarmored, err := armor.Decode(ciphertext)
	if err != nil {
		return
	}

	firstTimeCalled := true
	var prompt = func(keys []openpgp.Key, symmetric bool) ([]byte, error) {
		if firstTimeCalled {
			firstTimeCalled = false
			return password, nil
		}
		// Re-prompt still occurs if SKESK pasrsing fails (i.e. when decrypted cipher algo is invalid).
		// For most (but not all) cases, inputting a wrong passwords is expected to trigger this error.
		return nil, errors.New("gopenpgp: wrong password in symmetric decryption")
	}

	config := &packet.Config{
		DefaultCipher: packet.CipherAES256,
	}

	var emptyKeyRing openpgp.EntityList
	md, err := openpgp.ReadMessage(unarmored.Body, emptyKeyRing, prompt, config)
	if err != nil {
		// Parsing errors when reading the message are most likely caused by incorrect password, but we cannot know for sure
		return
	}

	plaintext = io.NopCloser(md.UnverifiedBody)

	return
}

// encrypt streams an OpenPGP-armored, symmetrically encrypted copy of
// plaintext through an io.Pipe. The pipe keeps memory use constant regardless
// of file size — the previous bytes.Buffer implementation retained roughly a
// third of the payload (the armor expansion) in RAM for the whole upload.
func encrypt(plaintext io.ReadCloser, password []byte) (ciphertext io.ReadCloser, err error) {
	config := &packet.Config{
		DefaultCipher: packet.CipherAES256,
		Time:          time.Now,
	}

	hints := &openpgp.FileHints{
		IsBinary: true,
		FileName: "",
		ModTime:  time.Unix(time.Now().Unix(), 0),
	}

	pr, pw := io.Pipe()

	go func() {
		armored, err := armor.Encode(pw, constants.PGPMessageHeader, nil)
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}

		encryptWriter, err := openpgp.SymmetricallyEncrypt(armored, password, hints, config)
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}

		if _, err := io.Copy(encryptWriter, plaintext); err != nil {
			_ = encryptWriter.Close()
			_ = armored.Close()
			_ = pw.CloseWithError(err)
			return
		}

		// Both closes flush trailing data, so a failure here means the
		// ciphertext is incomplete and must not be treated as success.
		if err := encryptWriter.Close(); err != nil {
			_ = armored.Close()
			_ = pw.CloseWithError(err)
			return
		}

		if err := armored.Close(); err != nil {
			_ = pw.CloseWithError(err)
			return
		}

		_ = pw.Close()
	}()

	return pr, nil
}

func canContainsXSS(contentType string) bool {
	switch {
	case strings.Contains(contentType, "cache-manifest"):
		fallthrough
	case strings.Contains(contentType, "html"):
		fallthrough
	case strings.Contains(contentType, "rdf"):
		fallthrough
	case strings.Contains(contentType, "vtt"):
		fallthrough
	case strings.Contains(contentType, "xml"):
		fallthrough
	case strings.Contains(contentType, "xsl"):
		return true
	case strings.Contains(contentType, "x-mixed-replace"):
		return true
	}

	return false
}

// this handler will output html or text, depending on the
// support of the client (Accept header).

func (s *Server) viewHandler(w http.ResponseWriter, r *http.Request) {
	// vars := mux.Vars(r)

	hostname := getURL(r, s.proxyPort).Host
	webAddress := resolveWebAddress(r, s.proxyPath, s.proxyPort)

	maxUploadSize := ""
	if s.maxUploadSize > 0 {
		maxUploadSize = formatSize(s.maxUploadSize)
	}

	purgeTime := ""
	if s.purgeDays > 0 {
		purgeTime = formatDurationDays(s.purgeDays)
	}

	data := struct {
		Hostname      string
		WebAddress    string
		EmailContact  string
		GAKey         string
		UserVoiceKey  string
		PurgeTime     string
		MaxUploadSize string
		SampleToken   string
		SampleToken2  string
	}{
		hostname,
		webAddress,
		s.emailContact,
		s.gaKey,
		s.userVoiceKey,
		purgeTime,
		maxUploadSize,
		token(s.randomTokenLength),
		token(s.randomTokenLength),
	}

	w.Header().Set("Vary", "Accept")
	if acceptsHTML(r.Header) {
		// If webPath is set and contains a static index.html, serve it directly
		// (supports Astro/SPA frontends that don't use Go templates)
		if s.webPath != "" {
			indexPath := filepath.Join(s.webPath, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				http.ServeFile(w, r, indexPath)
				return
			}
		}
		if err := htmlTemplates.ExecuteTemplate(w, "index.html", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if err := textTemplates.ExecuteTemplate(w, "index.txt", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (s *Server) notFoundHandler(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, http.StatusText(404), 404)
}

func sanitize(fileName string) string {
	t := transform.Chain(
		norm.NFD,
		runes.Remove(runes.In(unicode.Cc)),
		runes.Remove(runes.In(unicode.Cf)),
		runes.Remove(runes.In(unicode.Co)),
		runes.Remove(runes.In(unicode.Cs)),
		runes.Remove(runes.In(unicode.Other)),
		runes.Remove(runes.In(unicode.Zl)),
		runes.Remove(runes.In(unicode.Zp)),
		norm.NFC)
	newName, _, err := transform.String(t, fileName)
	if err != nil {
		return path.Base(fileName)
	}
	if len(newName) == 0 {
		newName = "_"
	}
	return path.Base(newName)
}

type metadata struct {
	// ContentType is the original uploading content type
	ContentType string
	// ContentLength is is the original uploading content length
	ContentLength int64
	// Downloads is the actual number of downloads
	Downloads int
	// MaxDownloads contains the maximum numbers of downloads
	MaxDownloads int
	// MaxDate contains the max age of the file
	MaxDate time.Time
	// DeletionToken contains the token to match against for deletion
	DeletionToken string
	// Encrypted contains if the file was encrypted
	Encrypted bool
	// DecryptedContentType is the original uploading content type
	DecryptedContentType string
}

func (metadata metadata) remainingLimitHeaderValues() (remainingDownloads, remainingDays string) {
	if metadata.MaxDate.IsZero() {
		remainingDays = "n/a"
	} else {
		timeDifference := time.Until(metadata.MaxDate)
		remainingDays = strconv.Itoa(int(timeDifference.Hours()/24) + 1)
	}

	if metadata.MaxDownloads == -1 {
		remainingDownloads = "n/a"
	} else {
		remainingDownloads = strconv.Itoa(metadata.MaxDownloads - metadata.Downloads)
	}

	return remainingDownloads, remainingDays
}

// fileLock is a per-upload mutex plus the number of goroutines currently
// holding or waiting for it, so the entry can be dropped once it is idle.
type fileLock struct {
	mu   sync.Mutex
	refs int
}

func (s *Server) lock(token, filename string) {
	key := path.Join(token, filename)

	s.locksMu.Lock()
	l, ok := s.locks[key]
	if !ok {
		l = &fileLock{}
		s.locks[key] = l
	}
	// Claim a reference before releasing locksMu so unlock() cannot delete
	// the entry out from under a goroutine that is about to wait on it.
	l.refs++
	s.locksMu.Unlock()

	l.mu.Lock()
}

func (s *Server) unlock(token, filename string) {
	key := path.Join(token, filename)

	s.locksMu.Lock()
	l, ok := s.locks[key]
	if !ok {
		s.locksMu.Unlock()
		return
	}
	l.refs--
	if l.refs == 0 {
		delete(s.locks, key)
	}
	s.locksMu.Unlock()

	l.mu.Unlock()
}

// checkMetadata loads and validates the metadata for an upload. Uploads that
// have exhausted their download or date limit are removed from storage rather
// than left behind for the (optional) scheduled purge to find later.
func (s *Server) checkMetadata(ctx context.Context, token, filename string) (metadata, error) {
	s.lock(token, filename)
	defer s.unlock(token, filename)

	return s.readMetadata(ctx, token, filename)
}

// readMetadata must be called with the token/filename lock held.
func (s *Server) readMetadata(ctx context.Context, token, filename string) (metadata, error) {
	var metadata metadata

	r, _, err := s.storage.Get(ctx, token, fmt.Sprintf("%s.metadata", filename), nil)
	defer storage.CloseCheck(r)

	if err != nil {
		return metadata, err
	}

	if err := json.NewDecoder(r).Decode(&metadata); err != nil {
		return metadata, err
	}

	if metadata.MaxDownloads != -1 && metadata.Downloads >= metadata.MaxDownloads {
		s.purgeExpired(ctx, token, filename)
		return metadata, errors.New("maxDownloads expired")
	}

	if !metadata.MaxDate.IsZero() && time.Now().After(metadata.MaxDate) {
		s.purgeExpired(ctx, token, filename)
		return metadata, errors.New("maxDate expired")
	}

	return metadata, nil
}

// increaseDownload records one *completed* download against the upload's
// Max-Downloads budget. It is deliberately called after the payload has been
// written, so aborted transfers, failed storage reads and link-preview fetches
// that never read the body do not burn a download.
func (s *Server) increaseDownload(ctx context.Context, token, filename string) error {
	s.lock(token, filename)
	defer s.unlock(token, filename)

	metadata, err := s.readMetadata(ctx, token, filename)
	if err != nil {
		return err
	}

	if metadata.MaxDownloads == -1 {
		return nil
	}

	metadata.Downloads++

	buffer := &bytes.Buffer{}
	if err := json.NewEncoder(buffer).Encode(metadata); err != nil {
		return errors.New("could not encode metadata")
	}

	if err := s.storage.Put(ctx, token, fmt.Sprintf("%s.metadata", filename), buffer, "text/json", uint64(buffer.Len())); err != nil {
		return errors.New("could not save metadata")
	}

	if metadata.Downloads >= metadata.MaxDownloads {
		s.purgeExpired(ctx, token, filename)
	}

	return nil
}

// purgeExpired removes an upload that has run out of downloads or days.
// Best effort: on failure the blob is simply left for the scheduled purge,
// so the error is logged instead of failing the request that noticed it.
func (s *Server) purgeExpired(ctx context.Context, token, filename string) {
	if err := s.storage.Delete(ctx, token, filename); err != nil && !s.storage.IsNotExist(err) {
		s.logger.Error("Could not delete expired upload", "token", token, "filename", filename, "error", err)
		return
	}

	s.metrics.expiredPurged.Add(1)
}

func (s *Server) checkDeletionToken(ctx context.Context, deletionToken, token, filename string) error {
	s.lock(token, filename)
	defer s.unlock(token, filename)

	var metadata metadata

	r, _, err := s.storage.Get(ctx, token, fmt.Sprintf("%s.metadata", filename), nil)
	defer storage.CloseCheck(r)

	if s.storage.IsNotExist(err) {
		return errors.New("metadata doesn't exist")
	} else if err != nil {
		return err
	}

	if err := json.NewDecoder(r).Decode(&metadata); err != nil {
		return err
	} else if metadata.DeletionToken != deletionToken {
		return errors.New("deletion token doesn't match")
	}

	return nil
}

func (s *Server) purgeHandler() {
	ticker := time.NewTicker(s.purgeInterval)
	go func() {
		for {
			<-ticker.C
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			err := s.storage.Purge(ctx, s.purgeDays)
			cancel()
			if err != nil {
				s.logger.Error("Error cleaning up expired files", "error", err)
			}
		}
	}()
}

func (s *Server) deleteHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	token := vars["token"]
	filename := vars["filename"]
	deletionToken := vars["deletionToken"]

	if err := s.checkDeletionToken(r.Context(), deletionToken, token, filename); err != nil {
		s.logger.Error("Error metadata", "error", err)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	err := s.storage.Delete(r.Context(), token, filename)
	if s.storage.IsNotExist(err) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	} else if err != nil {
		s.logger.Error("Error", "error", err)
		http.Error(w, "Could not delete file.", http.StatusInternalServerError)
		return
	}

	s.metrics.deletes.Add(1)
}

func resolveURL(r *http.Request, u *url.URL, proxyPort string) string {
	r.URL.Path = ""

	return getURL(r, proxyPort).ResolveReference(u).String()
}

func resolveKey(key, proxyPath string) string {
	key = strings.TrimPrefix(key, "/")

	key = strings.TrimPrefix(key, proxyPath)

	key = strings.ReplaceAll(key, "\\", "/")

	return key
}

func resolveWebAddress(r *http.Request, proxyPath string, proxyPort string) string {
	rUrl := getURL(r, proxyPort)

	var webAddress string

	if len(proxyPath) == 0 {
		webAddress = fmt.Sprintf("%s://%s/",
			rUrl.ResolveReference(rUrl).Scheme,
			rUrl.ResolveReference(rUrl).Host)
	} else {
		webAddress = fmt.Sprintf("%s://%s/%s",
			rUrl.ResolveReference(rUrl).Scheme,
			rUrl.ResolveReference(rUrl).Host,
			strings.TrimPrefix(proxyPath, "/"))
	}

	return webAddress
}

// Similar to the logic found here:
// https://github.com/golang/go/blob/release-branch.go1.14/src/net/http/clone.go#L22-L33
func cloneURL(u *url.URL) *url.URL {
	c := &url.URL{}
	*c = *u

	if u.User != nil {
		c.User = &url.Userinfo{}
		*c.User = *u.User
	}

	return c
}

func getURL(r *http.Request, proxyPort string) *url.URL {
	u := cloneURL(r.URL)

	if r.TLS != nil {
		u.Scheme = "https"
	} else if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		u.Scheme = proto
	} else {
		u.Scheme = "http"
	}

	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
		port = ""
	}

	p := idna.New(idna.ValidateForRegistration())
	var hostFromPunycode string
	hostFromPunycode, err = p.ToUnicode(host)
	if err == nil {
		host = hostFromPunycode
	}

	if len(proxyPort) != 0 {
		port = proxyPort
	}

	if len(port) == 0 {
		u.Host = host
	} else {
		if port == "80" && u.Scheme == "http" {
			u.Host = host
		} else if port == "443" && u.Scheme == "https" {
			u.Host = host
		} else {
			u.Host = net.JoinHostPort(host, port)
		}
	}

	return u
}

// isOwnURL reports whether raw is an absolute http(s) URL whose host matches
// the host this request came in on.
func (s *Server) isOwnURL(r *http.Request, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	return strings.EqualFold(u.Host, getURL(r, s.proxyPort).Host)
}

func commonHeader(w http.ResponseWriter, filename string) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Connection", "close")
	w.Header().Set("Cache-Control", "no-store")
}
