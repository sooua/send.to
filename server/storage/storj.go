package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"storj.io/common/fpath"
	"storj.io/common/storj"
	"storj.io/uplink"
)

// StorjStorage is a storage backed by Storj
type StorjStorage struct {
	Storage
	project   *uplink.Project
	bucket    *uplink.Bucket
	purgeDays time.Duration
	logger    *slog.Logger
}

// NewStorjStorage is the factory for StorjStorage
func NewStorjStorage(ctx context.Context, access, bucket string, purgeDays int, logger *slog.Logger) (*StorjStorage, error) {
	var instance StorjStorage
	var err error

	ctx = fpath.WithTempData(ctx, "", true)

	uplConf := &uplink.Config{
		UserAgent: "send-to",
	}

	parsedAccess, err := uplink.ParseAccess(access)
	if err != nil {
		return nil, err
	}

	instance.project, err = uplConf.OpenProject(ctx, parsedAccess)
	if err != nil {
		return nil, err
	}

	instance.bucket, err = instance.project.EnsureBucket(ctx, bucket)
	if err != nil {
		//Ignoring the error to return the one that occurred first, but try to clean up.
		_ = instance.project.Close()
		return nil, err
	}

	instance.purgeDays = time.Duration(purgeDays*24) * time.Hour

	instance.logger = logger

	return &instance, nil
}

// Type returns the storage type
func (s *StorjStorage) Type() string {
	return "storj"
}

// Head retrieves content length of a file from storage
func (s *StorjStorage) Head(ctx context.Context, token string, filename string) (contentLength uint64, err error) {
	key := storj.JoinPaths(token, filename)

	obj, err := s.project.StatObject(fpath.WithTempData(ctx, "", true), s.bucket.Name, key)
	if err != nil {
		return 0, err
	}

	contentLength = uint64(obj.System.ContentLength)

	return
}

// Get retrieves a file from storage
func (s *StorjStorage) Get(ctx context.Context, token string, filename string, rng *Range) (reader io.ReadCloser, contentLength uint64, err error) {
	key := storj.JoinPaths(token, filename)

	s.logger.Info("Getting file from Storj Bucket", "filename", filename)

	var options *uplink.DownloadOptions
	if rng != nil {
		options = new(uplink.DownloadOptions)
		options.Offset = int64(rng.Start)
		if rng.Limit > 0 {
			options.Length = int64(rng.Limit)
		} else {
			options.Length = -1
		}
	}

	download, err := s.project.DownloadObject(fpath.WithTempData(ctx, "", true), s.bucket.Name, key, options)
	if err != nil {
		return nil, 0, err
	}

	contentLength = uint64(download.Info().System.ContentLength)
	if rng != nil {
		contentLength = rng.AcceptLength(contentLength)
	}

	reader = download
	return
}

// Delete removes a file and its metadata sidecar from storage.
func (s *StorjStorage) Delete(ctx context.Context, token string, filename string) (err error) {
	s.logger.Info("Deleting file from Storj Bucket", "filename", filename)

	// Metadata removal is best effort, matching the local and S3 backends:
	// leaving a stale sidecar behind would make the token look alive.
	metadataKey := storj.JoinPaths(token, fmt.Sprintf("%s.metadata", filename))
	_, _ = s.project.DeleteObject(fpath.WithTempData(ctx, "", true), s.bucket.Name, metadataKey)

	key := storj.JoinPaths(token, filename)
	_, err = s.project.DeleteObject(fpath.WithTempData(ctx, "", true), s.bucket.Name, key)

	return
}

// Purge cleans up the storage
func (s *StorjStorage) Purge(context.Context, time.Duration) (err error) {
	// NOOP expiration is set at upload time
	return nil
}

// Put saves a file on storage
func (s *StorjStorage) Put(ctx context.Context, token string, filename string, reader io.Reader, contentType string, contentLength uint64) (err error) {
	key := storj.JoinPaths(token, filename)

	s.logger.Info("Uploading file to Storj Bucket", "filename", filename)

	var uploadOptions *uplink.UploadOptions
	if s.purgeDays.Hours() > 0 {
		uploadOptions = &uplink.UploadOptions{Expires: time.Now().Add(s.purgeDays)}
	}

	writer, err := s.project.UploadObject(fpath.WithTempData(ctx, "", true), s.bucket.Name, key, uploadOptions)
	if err != nil {
		return err
	}

	n, err := io.Copy(writer, reader)
	// contentLength == 0 means "length unknown up front" — encrypted uploads
	// stream ciphertext whose size is not known until the last byte, so only
	// verify when the caller actually declared a length.
	if err != nil || (contentLength > 0 && uint64(n) != contentLength) {
		//Ignoring the error to return the one that occurred first, but try to clean up.
		_ = writer.Abort()
		if err == nil {
			err = fmt.Errorf("storj: wrote %d bytes, expected %d", n, contentLength)
		}
		return err
	}
	err = writer.SetCustomMetadata(ctx, uplink.CustomMetadata{"content-type": contentType})
	if err != nil {
		//Ignoring the error to return the one that occurred first, but try to clean up.
		_ = writer.Abort()
		return err
	}

	err = writer.Commit()
	return err
}

// Usage is not implemented for Storj: counting the bucket means listing it
// with system metadata, and this fork has no Storj credentials to verify such
// code against. Reporting the limitation is honest; guessing at an untested
// listing call is not.
func (s *StorjStorage) Usage(context.Context) (uint64, error) {
	return 0, ErrUsageUnsupported
}

func (s *StorjStorage) IsRangeSupported() bool { return true }

// IsNotExist indicates if a file doesn't exist on storage
func (s *StorjStorage) IsNotExist(err error) bool {
	return errors.Is(err, uplink.ErrObjectNotFound)
}
