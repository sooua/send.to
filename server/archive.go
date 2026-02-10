package server

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sooua/send.to/server/storage"
	"github.com/gorilla/mux"
)

func (s *Server) zipHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	files := vars["files"]

	zipfilename := fmt.Sprintf("sendto-%d.zip", uint16(time.Now().UnixNano()))

	w.Header().Set("Content-Type", "application/zip")
	commonHeader(w, zipfilename)

	zw := zip.NewWriter(w)

	for _, key := range strings.Split(files, ",") {
		key = resolveKey(key, s.proxyPath)

		token := strings.Split(key, "/")[0]
		filename := sanitize(strings.Split(key, "/")[1])

		if _, err := s.checkMetadata(r.Context(), token, filename, true); err != nil {
			s.logger.Error("Error metadata", "error", err)
			continue
		}

		reader, _, err := s.storage.Get(r.Context(), token, filename, nil)
		defer storage.CloseCheck(reader)

		if err != nil {
			if s.storage.IsNotExist(err) {
				http.Error(w, "File not found", 404)
				return
			}

			s.logger.Error("Error", "error", err)
			http.Error(w, "Could not retrieve file.", http.StatusInternalServerError)
			return
		}

		header := &zip.FileHeader{
			Name:   strings.Split(key, "/")[1],
			Method: zip.Store,

			Modified: time.Now().UTC(),
		}

		fw, err := zw.CreateHeader(header)

		if err != nil {
			s.logger.Error("Error", "error", err)
			http.Error(w, "Internal server error.", http.StatusInternalServerError)
			return
		}

		if _, err = io.Copy(fw, reader); err != nil {
			s.logger.Error("Error", "error", err)
			http.Error(w, "Internal server error.", http.StatusInternalServerError)
			return
		}
	}

	if err := zw.Close(); err != nil {
		s.logger.Error("Error", "error", err)
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return
	}
}

func (s *Server) tarGzHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	files := vars["files"]

	tarfilename := fmt.Sprintf("sendto-%d.tar.gz", uint16(time.Now().UnixNano()))

	w.Header().Set("Content-Type", "application/x-gzip")
	commonHeader(w, tarfilename)

	gw := gzip.NewWriter(w)
	defer storage.CloseCheck(gw)

	zw := tar.NewWriter(gw)
	defer storage.CloseCheck(zw)

	for _, key := range strings.Split(files, ",") {
		key = resolveKey(key, s.proxyPath)

		token := strings.Split(key, "/")[0]
		filename := sanitize(strings.Split(key, "/")[1])

		if _, err := s.checkMetadata(r.Context(), token, filename, true); err != nil {
			s.logger.Error("Error metadata", "error", err)
			continue
		}

		reader, contentLength, err := s.storage.Get(r.Context(), token, filename, nil)
		defer storage.CloseCheck(reader)

		if err != nil {
			if s.storage.IsNotExist(err) {
				http.Error(w, "File not found", 404)
				return
			}

			s.logger.Error("Error", "error", err)
			http.Error(w, "Could not retrieve file.", http.StatusInternalServerError)
			return
		}

		header := &tar.Header{
			Name: strings.Split(key, "/")[1],
			Size: int64(contentLength),
		}

		err = zw.WriteHeader(header)
		if err != nil {
			s.logger.Error("Error", "error", err)
			http.Error(w, "Internal server error.", http.StatusInternalServerError)
			return
		}

		if _, err = io.Copy(zw, reader); err != nil {
			s.logger.Error("Error", "error", err)
			http.Error(w, "Internal server error.", http.StatusInternalServerError)
			return
		}
	}
}

func (s *Server) tarHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	files := vars["files"]

	tarfilename := fmt.Sprintf("sendto-%d.tar", uint16(time.Now().UnixNano()))

	w.Header().Set("Content-Type", "application/x-tar")
	commonHeader(w, tarfilename)

	zw := tar.NewWriter(w)
	defer storage.CloseCheck(zw)

	for _, key := range strings.Split(files, ",") {
		key = resolveKey(key, s.proxyPath)

		token := strings.Split(key, "/")[0]
		filename := strings.Split(key, "/")[1]

		if _, err := s.checkMetadata(r.Context(), token, filename, true); err != nil {
			s.logger.Error("Error metadata", "error", err)
			continue
		}

		reader, contentLength, err := s.storage.Get(r.Context(), token, filename, nil)
		defer storage.CloseCheck(reader)

		if err != nil {
			if s.storage.IsNotExist(err) {
				http.Error(w, "File not found", 404)
				return
			}

			s.logger.Error("Error", "error", err)
			http.Error(w, "Could not retrieve file.", http.StatusInternalServerError)
			return
		}

		header := &tar.Header{
			Name: strings.Split(key, "/")[1],
			Size: int64(contentLength),
		}

		err = zw.WriteHeader(header)
		if err != nil {
			s.logger.Error("Error", "error", err)
			http.Error(w, "Internal server error.", http.StatusInternalServerError)
			return
		}

		if _, err = io.Copy(zw, reader); err != nil {
			s.logger.Error("Error", "error", err)
			http.Error(w, "Internal server error.", http.StatusInternalServerError)
			return
		}
	}
}
