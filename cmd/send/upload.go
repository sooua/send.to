package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sooua/send.to/client"
)

// uploadResumable sends a file in chunks the server accepts one at a time, and
// remembers the session so a run that dies — or a link that does — continues
// from where it stopped instead of from zero.
//
// It returns client.ErrResumableUnsupported when the server has no such
// endpoint, so the caller can fall back to a plain PUT.
func uploadResumable(ctx context.Context, api *client.Client, server, argPath string, f *os.File, name string, size int64, opts client.UploadOptions, quiet, e2e bool) (*client.Result, error) {
	if e2e && opts.Password != "" {
		return nil, errors.New("--e2e and --encrypt are alternatives: --e2e keeps the key on this machine, --encrypt hands it to the server")
	}

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(argPath)
	if err != nil {
		absPath = argPath
	}

	resumeKey := client.PendingKey(server, absPath, size, info.ModTime())

	pending, err := client.LoadPending()
	if err != nil {
		pending = &client.Pending{}
	}

	sess, stream := resumeSession(ctx, api, pending, resumeKey, name, e2e)

	if sess == nil {
		pending.Remove(resumeKey)

		length := size

		if e2e {
			key, err := client.NewE2EKey()
			if err != nil {
				return nil, err
			}

			if stream, err = client.NewE2EStream(key, e2eMetadataFor(name)); err != nil {
				return nil, err
			}

			if length, err = stream.Size(size); err != nil {
				return nil, err
			}
		}

		if sess, err = api.CreateSession(ctx, name, length, opts); err != nil {
			return nil, err
		}

		entry := client.PendingUpload{
			Key:        resumeKey,
			SessionURL: sess.URL,
			Server:     server,
			Path:       absPath,
			Name:       name,
			Size:       size,
			Length:     length,
			StartedAt:  time.Now(),
		}
		if stream != nil {
			entry.E2EKey = stream.Key.String()
			entry.E2EPrefix = base64.RawURLEncoding.EncodeToString(stream.Prefix)
		}
		pending.Put(entry)
		if err := pending.Save(); err != nil {
			fmt.Fprintln(os.Stderr, "warning: could not record the upload session:", err)
		}
	}

	src := func(offset int64) (io.ReadCloser, error) {
		if stream != nil {
			return stream.ReaderFrom(f, offset)
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, err
		}
		return io.NopCloser(f), nil
	}

	var progress func(int64)

	if !quiet {
		if sess.Offset > 0 {
			fmt.Fprintf(os.Stderr, "resuming at %s of %s\n", humanBytes(sess.Offset), humanBytes(sess.Length))
		}

		bar := newProgressBar(name, sess.Length)
		defer bar.done()
		progress = bar.set
	}

	result, err := api.UploadSession(ctx, sess, src, progress)
	if err != nil {
		// The session is still on the server, so say so rather than leaving
		// the user to guess whether anything survived.
		return nil, fmt.Errorf("%w (run the same command again to resume)", err)
	}

	pending.Remove(resumeKey)
	if err := pending.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not clear the upload session record:", err)
	}

	if stream != nil {
		result.URL += "#k=" + stream.Key.String()
		result.Encrypted = true
		result.Size = size
	}

	return result, nil
}

// resumeSession revives a recorded session if the server still has it, together
// with the encryption stream that produced the bytes it already holds.
func resumeSession(ctx context.Context, api *client.Client, pending *client.Pending, resumeKey, name string, e2e bool) (*client.Session, *client.E2EStream) {
	entry := pending.Find(resumeKey)
	if entry == nil {
		return nil, nil
	}

	// An entry from an --e2e run cannot continue a plain one, or the reverse:
	// the byte stream is a different length and different content.
	if (entry.E2EKey != "") != e2e {
		return nil, nil
	}

	var stream *client.E2EStream

	if e2e {
		key, err := client.ParseE2EKey(entry.E2EKey)
		if err != nil {
			return nil, nil
		}

		prefix, err := base64.RawURLEncoding.DecodeString(entry.E2EPrefix)
		if err != nil {
			return nil, nil
		}

		if stream, err = client.NewE2EStreamWithPrefix(key, prefix, e2eMetadataFor(name)); err != nil {
			return nil, nil
		}
	}

	status, err := api.SessionStatus(ctx, entry.SessionURL)
	if err != nil {
		return nil, nil
	}

	if status.Length != entry.Length {
		return nil, nil
	}

	return status, stream
}

func e2eMetadataFor(name string) client.E2EMetadata {
	return client.E2EMetadata{Name: name, Type: contentTypeFor(name)}
}
