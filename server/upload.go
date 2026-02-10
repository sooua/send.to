package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sooua/send.to/server/storage"
	"github.com/gorilla/mux"
)

func (s *Server) postHandler(w http.ResponseWriter, r *http.Request) {
	if s.maxUploadSize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadSize)
	}

	if err := r.ParseMultipartForm(_24K); nil != err {
		s.logger.Error("Error parsing multipart form", "error", err)
		http.Error(w, "Error occurred copying to output stream", http.StatusInternalServerError)
		return
	}

	token := token(s.randomTokenLength)

	w.Header().Set("Content-Type", "text/plain")

	responseBody := ""

	for _, fHeaders := range r.MultipartForm.File {
		for _, fHeader := range fHeaders {
			filename := sanitize(fHeader.Filename)
			contentType := mime.TypeByExtension(filepath.Ext(fHeader.Filename))

			var f io.Reader
			var err error

			if f, err = fHeader.Open(); err != nil {
				s.logger.Error("Error opening uploaded file", "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			file, err := os.CreateTemp(s.tempPath, "transfer-")
			defer s.cleanTmpFile(file)

			if err != nil {
				s.logger.Error("Error", "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			n, err := io.Copy(file, f)
			if err != nil {
				s.logger.Error("Error", "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			contentLength := n

			_, err = file.Seek(0, io.SeekStart)
			if err != nil {
				s.logger.Error("Error", "error", err)
				return
			}

			if s.maxUploadSize > 0 && contentLength > s.maxUploadSize {
				s.logger.Warn("Entity too large")
				http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
				return
			}

			if s.performClamavPrescan {
				status, err := s.performScan(file.Name())
				if err != nil {
					s.logger.Error("Error", "error", err)
					http.Error(w, "Could not perform prescan", http.StatusInternalServerError)
					return
				}

				if status != clamavScanStatusOK {
					s.logger.Warn("Clamav prescan positive", "status", status)
					http.Error(w, "Clamav prescan found a virus", http.StatusPreconditionFailed)
					return
				}
			}

			metadata := metadataForRequest(contentType, contentLength, s.randomTokenLength, r)

			buffer := &bytes.Buffer{}
			if err := json.NewEncoder(buffer).Encode(metadata); err != nil {
				s.logger.Error("Error", "error", err)
				http.Error(w, "Could not encode metadata", http.StatusInternalServerError)

				return
			} else if err := s.storage.Put(r.Context(), token, fmt.Sprintf("%s.metadata", filename), buffer, "text/json", uint64(buffer.Len())); err != nil {
				s.logger.Error("Error", "error", err)
				http.Error(w, "Could not save metadata", http.StatusInternalServerError)

				return
			}

			s.logger.Info("Uploading", "token", token, "filename", filename, "content_length", contentLength, "content_type", contentType)

			reader, err := attachEncryptionReader(file, r.Header.Get("X-Encrypt-Password"))
			if err != nil {
				http.Error(w, "Could not crypt file", http.StatusInternalServerError)
				return
			}

			if err = s.storage.Put(r.Context(), token, filename, reader, contentType, uint64(contentLength)); err != nil {
				s.logger.Error("Backend storage error", "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return

			}

			filename = url.PathEscape(filename)
			relativeURL, _ := url.Parse(path.Join(s.proxyPath, token, filename))
			deleteURL, _ := url.Parse(path.Join(s.proxyPath, token, filename, metadata.DeletionToken))
			w.Header().Add("X-Url-Delete", resolveURL(r, deleteURL, s.proxyPort))
			responseBody += fmt.Sprintln(getURL(r, s.proxyPort).ResolveReference(relativeURL).String())
		}
	}
	_, err := w.Write([]byte(responseBody))
	if err != nil {
		s.logger.Error("Error", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) putHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	filename := sanitize(vars["filename"])

	contentLength := r.ContentLength

	if s.maxUploadSize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadSize)
	}

	defer storage.CloseCheck(r.Body)

	reader := r.Body

	if contentLength < 1 || s.performClamavPrescan {
		file, err := os.CreateTemp(s.tempPath, "transfer-")
		defer s.cleanTmpFile(file)
		if err != nil {
			s.logger.Error("Error", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// queue file to disk, because s3 needs content length
		// and clamav prescan scans a file
		n, err := io.Copy(file, r.Body)
		if err != nil {
			s.logger.Error("Error", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			s.logger.Error("Error", "error", err)
			http.Error(w, "Cannot reset cache file", http.StatusInternalServerError)

			return
		}

		contentLength = n

		if s.performClamavPrescan {
			status, err := s.performScan(file.Name())
			if err != nil {
				s.logger.Error("Error", "error", err)
				http.Error(w, "Could not perform prescan", http.StatusInternalServerError)
				return
			}

			if status != clamavScanStatusOK {
				s.logger.Warn("Clamav prescan positive", "status", status)
				http.Error(w, "Clamav prescan found a virus", http.StatusPreconditionFailed)
				return
			}
		}

		reader = file
	}

	if s.maxUploadSize > 0 && contentLength > s.maxUploadSize {
		s.logger.Warn("Entity too large")
		http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return
	}

	if contentLength == 0 {
		s.logger.Warn("Empty content-length")
		http.Error(w, "Could not upload empty file", http.StatusBadRequest)
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(vars["filename"]))

	token := token(s.randomTokenLength)

	metadata := metadataForRequest(contentType, contentLength, s.randomTokenLength, r)

	buffer := &bytes.Buffer{}
	if err := json.NewEncoder(buffer).Encode(metadata); err != nil {
		s.logger.Error("Error", "error", err)
		http.Error(w, "Could not encode metadata", http.StatusInternalServerError)
		return
	} else if !metadata.MaxDate.IsZero() && time.Now().After(metadata.MaxDate) {
		s.logger.Warn("Invalid MaxDate")
		http.Error(w, "Invalid MaxDate, make sure Max-Days is smaller than 290 years", http.StatusBadRequest)
		return
	} else if err := s.storage.Put(r.Context(), token, fmt.Sprintf("%s.metadata", filename), buffer, "text/json", uint64(buffer.Len())); err != nil {
		s.logger.Error("Error", "error", err)
		http.Error(w, "Could not save metadata", http.StatusInternalServerError)
		return
	}

	s.logger.Info("Uploading", "token", token, "filename", filename, "content_length", contentLength, "content_type", contentType)

	reader, err := attachEncryptionReader(reader, r.Header.Get("X-Encrypt-Password"))
	if err != nil {
		http.Error(w, "Could not crypt file", http.StatusInternalServerError)
		return
	}

	if err = s.storage.Put(r.Context(), token, filename, reader, contentType, uint64(contentLength)); err != nil {
		s.logger.Error("Error putting new file", "error", err)
		http.Error(w, "Could not save file", http.StatusInternalServerError)
		return
	}

	// w.Statuscode = 200

	w.Header().Set("Content-Type", "text/plain")

	filename = url.PathEscape(filename)
	relativeURL, _ := url.Parse(path.Join(s.proxyPath, token, filename))
	deleteURL, _ := url.Parse(path.Join(s.proxyPath, token, filename, metadata.DeletionToken))

	w.Header().Set("X-Url-Delete", resolveURL(r, deleteURL, s.proxyPort))

	_, _ = w.Write([]byte(resolveURL(r, relativeURL, s.proxyPort)))
}

func (s *Server) cleanTmpFile(f *os.File) {
	if f != nil {
		err := f.Close()
		if err != nil {
			s.logger.Error("Error closing tmpfile", "error", err, "file", f.Name())
		}

		err = os.Remove(f.Name())
		if err != nil {
			s.logger.Error("Error removing tmpfile", "error", err, "file", f.Name())
		}
	}
}

func metadataForRequest(contentType string, contentLength int64, randomTokenLength int, r *http.Request) metadata {
	metadata := metadata{
		ContentType:   strings.ToLower(contentType),
		ContentLength: contentLength,
		MaxDate:       time.Time{},
		Downloads:     0,
		MaxDownloads:  -1,
		DeletionToken: token(randomTokenLength) + token(randomTokenLength),
	}

	if v := r.Header.Get("Max-Downloads"); v == "" {
	} else if v, err := strconv.Atoi(v); err != nil {
	} else {
		metadata.MaxDownloads = v
	}

	if v := r.Header.Get("Max-Days"); v == "" {
	} else if v, err := strconv.Atoi(v); err != nil {
	} else {
		metadata.MaxDate = time.Now().Add(time.Hour * 24 * time.Duration(v))
	}

	if password := r.Header.Get("X-Encrypt-Password"); password != "" {
		metadata.Encrypted = true
		metadata.ContentType = "text/plain; charset=utf-8"
		metadata.DecryptedContentType = contentType
	} else {
		metadata.Encrypted = false
	}

	return metadata
}
