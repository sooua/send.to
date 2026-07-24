package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Storage is a storage backed by AWS S3
type S3Storage struct {
	Storage
	bucket      string
	s3          *s3.Client
	logger      *slog.Logger
	purgeDays   time.Duration
	noMultipart bool
}

// NewS3Storage is the factory for S3Storage
func NewS3Storage(ctx context.Context, accessKey, secretKey, bucketName string, purgeDays int, region, endpoint string, disableMultipart bool, forcePathStyle bool, logger *slog.Logger) (*S3Storage, error) {
	cfg, err := getAwsConfig(ctx, accessKey, secretKey)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.Region = region
		o.UsePathStyle = forcePathStyle
		if len(endpoint) > 0 {
			o.EndpointResolver = s3.EndpointResolverFromURL(endpoint)
		}
	})

	return &S3Storage{
		bucket:      bucketName,
		s3:          client,
		logger:      logger,
		noMultipart: disableMultipart,
		purgeDays:   time.Duration(purgeDays*24) * time.Hour,
	}, nil
}

// Type returns the storage type
func (s *S3Storage) Type() string {
	return "s3"
}

// Head retrieves content length of a file from storage
func (s *S3Storage) Head(ctx context.Context, token string, filename string) (contentLength uint64, err error) {
	key := fmt.Sprintf("%s/%s", token, filename)

	headRequest := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	// content type , content length
	response, err := s.s3.HeadObject(ctx, headRequest)
	if err != nil {
		return
	}

	if response.ContentLength != nil {
		contentLength = uint64(*response.ContentLength)
	}

	return
}

// Purge deletes every object last modified before the cutoff.
//
// This used to be a no-op justified by "expiration is set at upload time",
// which was a misreading of the SDK: PutObjectInput.Expires is the HTTP
// Expires header from RFC 7234 — a caching hint. S3 deletes nothing because of
// it. An operator running --purge-days against S3 was told files were being
// removed after N days while every one of them was kept forever.
//
// Listing the bucket is the only way to find them, so purge costs one
// ListObjectsV2 page per 1000 objects. Deletes go out in batches of the same
// size, which is also DeleteObjects' limit.
func (s *S3Storage) Purge(ctx context.Context, days time.Duration) error {
	cutoff := time.Now().Add(-days)

	paginator := s3.NewListObjectsV2Paginator(s.s3, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		expired := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, object := range page.Contents {
			if object.Key == nil || object.LastModified == nil {
				continue
			}
			if object.LastModified.Before(cutoff) {
				expired = append(expired, types.ObjectIdentifier{Key: object.Key})
			}
		}

		if len(expired) == 0 {
			continue
		}

		response, err := s.s3.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{Objects: expired, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return err
		}

		// A per-key failure does not fail the sweep — the next one retries it —
		// but it must be visible, otherwise the bucket quietly stops shrinking.
		for _, failure := range response.Errors {
			s.logger.Error("Could not purge object",
				"key", aws.ToString(failure.Key), "error", aws.ToString(failure.Message))
		}
	}

	return nil
}

// IsNotExist indicates if a file doesn't exist on storage
func (s *S3Storage) IsNotExist(err error) bool {
	if err == nil {
		return false
	}

	var nkerr *types.NoSuchKey
	return errors.As(err, &nkerr)
}

// Get retrieves a file from storage
func (s *S3Storage) Get(ctx context.Context, token string, filename string, rng *Range) (reader io.ReadCloser, contentLength uint64, err error) {
	key := fmt.Sprintf("%s/%s", token, filename)

	getRequest := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	if rng != nil {
		getRequest.Range = aws.String(rng.Range())
	}

	response, err := s.s3.GetObject(ctx, getRequest)
	if err != nil {
		return
	}

	if response.ContentLength != nil {
		contentLength = uint64(*response.ContentLength)
	}
	if rng != nil && response.ContentRange != nil {
		rng.SetContentRange(*response.ContentRange)
	}

	reader = response.Body
	return
}

// Delete removes a file from storage
func (s *S3Storage) Delete(ctx context.Context, token string, filename string) (err error) {
	metadata := fmt.Sprintf("%s/%s.metadata", token, filename)
	deleteRequest := &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(metadata),
	}

	_, err = s.s3.DeleteObject(ctx, deleteRequest)
	if err != nil {
		return
	}

	key := fmt.Sprintf("%s/%s", token, filename)
	deleteRequest = &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	_, err = s.s3.DeleteObject(ctx, deleteRequest)

	return
}

// Put saves a file on storage
func (s *S3Storage) Put(ctx context.Context, token string, filename string, reader io.Reader, contentType string, _ uint64) (err error) {
	key := fmt.Sprintf("%s/%s", token, filename)

	s.logger.Info("Uploading file to S3 Bucket", "filename", filename)
	var concurrency int
	if !s.noMultipart {
		concurrency = 20
	} else {
		concurrency = 1
	}

	// Create an uploader with the session and custom options
	uploader := manager.NewUploader(s.s3, func(u *manager.Uploader) {
		u.Concurrency = concurrency // default is 5
		u.LeavePartsOnError = false
	})

	// Expires is the HTTP caching header, not an object lifetime: it tells
	// caches when the object stops being worth keeping, and the purge sweep is
	// what actually removes it.
	var expire *time.Time
	if s.purgeDays.Hours() > 0 {
		expire = aws.Time(time.Now().Add(s.purgeDays))
	}

	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        reader,
		Expires:     expire,
		ContentType: aws.String(contentType),
	})

	return
}

// Usage sums the bucket, one ListObjectsV2 page at a time. It is called once at
// startup: on a large bucket this is thousands of keys, but the alternative —
// a quota that silently does nothing — is worse.
func (s *S3Storage) Usage(ctx context.Context) (uint64, error) {
	var total uint64

	paginator := s3.NewListObjectsV2Paginator(s.s3, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return 0, err
		}

		for _, object := range page.Contents {
			if object.Size != nil {
				total += uint64(*object.Size)
			}
		}
	}

	return total, nil
}

func (s *S3Storage) IsRangeSupported() bool { return true }

func getAwsConfig(ctx context.Context, accessKey, secretKey string) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     accessKey,
				SecretAccessKey: secretKey,
				SessionToken:    "",
			},
		}),
	)
}
