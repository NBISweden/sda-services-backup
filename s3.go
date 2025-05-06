package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	log "github.com/sirupsen/logrus"
)

type s3Backend struct {
	Client     *s3.S3
	Uploader   *s3manager.Uploader
	Bucket     string
	PathPrefix string
}

// S3Config stores information about the S3 storage backend
type S3Config struct {
	URL        string
	Port       int
	AccessKey  string
	SecretKey  string
	Bucket     string
	Region     string
	Chunksize  int
	Cacert     string
	PathPrefix string
}

func newS3Backend(config S3Config) (*s3Backend, error) {
	log.Info("Start initializing the S3 backend")
	s3Transport := transportConfigS3(config)
	client := http.Client{Transport: s3Transport}
	s3Session := session.Must(session.NewSession(
		&aws.Config{
			Endpoint:         aws.String(fmt.Sprintf("%s:%d", config.URL, config.Port)),
			Region:           aws.String(config.Region),
			HTTPClient:       &client,
			S3ForcePathStyle: aws.Bool(true),
			DisableSSL:       aws.Bool(strings.HasPrefix(config.URL, "http:")),
			Credentials:      credentials.NewStaticCredentials(config.AccessKey, config.SecretKey, ""),
		},
	))

	_, err := s3.New(s3Session).CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(config.Bucket),
	})

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {

			if aerr.Code() != s3.ErrCodeBucketAlreadyOwnedByYou &&
				aerr.Code() != s3.ErrCodeBucketAlreadyExists {
				log.Error("Unexpected issue while creating bucket", err)
			}
		}
	}

	sb := &s3Backend{
		Bucket: config.Bucket,
		Uploader: s3manager.NewUploader(s3Session, func(u *s3manager.Uploader) {
			u.LeavePartsOnError = false
		}),
		Client:     s3.New(s3Session),
		PathPrefix: config.PathPrefix,
	}

	_, err = sb.Client.ListObjectsV2(&s3.ListObjectsV2Input{Bucket: &config.Bucket})
	if err != nil {
		return nil, err
	}

	return sb, nil
}

// NewFileReader returns an io.Reader instance
func (sb *s3Backend) NewFileReader(filePath string) (io.ReadCloser, error) {
	if sb == nil {
		return nil, fmt.Errorf("Invalid s3Backend")
	}

	r, err := sb.Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(sb.Bucket),
		Key:    aws.String(filePath),
	})

	if err != nil {
		log.Error("Failed to retrieve s3 content")

		return nil, err
	}

	log.Infof("Retrieved backup content len: %v", *r.ContentLength)

	return r.Body, nil
}

// NewFileWriter uploads the contents of an io.Reader to a S3 bucket
func (sb *s3Backend) NewFileWriter(filePath string, wg *sync.WaitGroup) (io.WriteCloser, error) {
	if sb == nil {
		return nil, fmt.Errorf("Invalid s3Backend")
	}

	reader, writer := io.Pipe()
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := sb.Uploader.Upload(&s3manager.UploadInput{
			Body:            reader,
			Bucket:          aws.String(sb.Bucket),
			Key:             aws.String(filePath),
			ContentEncoding: aws.String("application/octet-stream"),
		})

		if err != nil {
			_ = reader.CloseWithError(err)
		}
	}()

	return writer, nil
}

// transportConfigS3 is a helper method to setup TLS for the S3 client.
func transportConfigS3(config S3Config) http.RoundTripper {
	cfg := new(tls.Config)

	// Enforce TLS1.2 or higher
	cfg.MinVersion = 2

	// Read system CAs
	var systemCAs, _ = x509.SystemCertPool()
	if reflect.DeepEqual(systemCAs, x509.NewCertPool()) {
		log.Debug("creating new CApool")
		systemCAs = x509.NewCertPool()
	}
	cfg.RootCAs = systemCAs

	if config.Cacert != "" {
		cacert, e := os.ReadFile(config.Cacert) // #nosec this file comes from our config
		if e != nil {
			log.Fatalf("failed to append %q to RootCAs: %v", cacert, e)
		}
		if ok := cfg.RootCAs.AppendCertsFromPEM(cacert); !ok {
			log.Debug("no certs appended, using system certs only")
		}
	}

	var trConfig http.RoundTripper = &http.Transport{
		TLSClientConfig:   cfg,
		ForceAttemptHTTP2: true}

	return trConfig
}

func BackupS3BucketEncrypted(source, destination *s3Backend, publicKeyPath string) error {
	privateKey, publicKeyList, err := getKeys(publicKeyPath)
	if err != nil {
		return fmt.Errorf("could not retrieve public key or generate private key: %s", err)
	}

	// list files in src bucket
	result, err := source.Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: &source.Bucket,
		Prefix: &source.PathPrefix,
	})
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	for _, obj := range result.Contents {
		log.Debugf("copying object: %s", *obj.Key)
		s, err := source.Client.GetObject(&s3.GetObjectInput{
			Bucket: &source.Bucket,
			Key:    obj.Key,
		})
		if err != nil {
			return err
		}
		defer s.Body.Close()

		wr, err := destination.NewFileWriter(fmt.Sprintf("%s.c4gh", *obj.Key), &wg)
		if err != nil {
			return fmt.Errorf("could not open backup writer: %s", err)
		}

		e, err := newEncryptor(publicKeyList, privateKey, wr)
		if err != nil {
			return err
		}

		i, err := io.Copy(e, s.Body)
		if err != nil {
			return fmt.Errorf("failed to copy data: %s", err.Error())
		}
		log.Debugf("bytes copied: %d", i)
		err = e.Close()
		if err != nil {
			return err
		}

		err = wr.Close()
		if err != nil {
			return err
		}
	}
	wg.Wait()

	return nil
}

func RestoreEncryptedS3Bucket(source, destination *s3Backend, passphrase, privateKeyPath string) error {
	privateKey, err := getPrivateKey(privateKeyPath, passphrase)
	if err != nil {
		return fmt.Errorf("private key error: %s", err)
	}

	// list files in src bucket
	result, err := source.Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: &source.Bucket,
		Prefix: &source.PathPrefix,
	})
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	for _, obj := range result.Contents {
		wg.Add(1)
		log.Debugf("restoring object: %s", *obj.Key)
		s, err := source.Client.GetObject(&s3.GetObjectInput{
			Bucket: &source.Bucket,
			Key:    obj.Key,
		})
		if err != nil {
			return err
		}
		defer s.Body.Close()

		reader, wr := io.Pipe()
		go func() {
			defer wg.Done()
			_, err := destination.Uploader.Upload(&s3manager.UploadInput{
				Body:            reader,
				Bucket:          aws.String(destination.Bucket),
				Key:             aws.String(strings.TrimSuffix(*obj.Key, ".c4gh")),
				ContentEncoding: aws.String("application/octet-stream"),
			})
			if err != nil {
				_ = reader.CloseWithError(err)
			}
		}()

		d, err := newDecryptor(privateKey, s.Body)
		if err != nil {
			log.Error("c4gh decryptor failure")

			return err
		}

		i, err := io.Copy(wr, d)
		if err != nil {
			return fmt.Errorf("failed to copy data: %s", err.Error())
		}
		log.Debugf("bytes copied: %d", i)

		err = d.Close()
		if err != nil {
			return err
		}

		err = wr.Close()
		if err != nil {
			return err
		}
	}
	wg.Wait()

	return nil
}

func SyncS3Buckets(source, destination *s3Backend) error {
	result, err := source.Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: &source.Bucket,
		Prefix: &source.PathPrefix,
	})
	if err != nil {
		return err
	}

	for _, obj := range result.Contents {
		log.Debugf("copying object: %s", *obj.Key)
		s, err := source.Client.GetObject(&s3.GetObjectInput{
			Bucket: &source.Bucket,
			Key:    obj.Key,
		})
		if err != nil {
			return err
		}
		defer s.Body.Close()

		_, err = destination.Uploader.Upload(&s3manager.UploadInput{
			Body:            s.Body,
			Bucket:          aws.String(destination.Bucket),
			Key:             obj.Key,
			ContentEncoding: aws.String("application/octet-stream"),
		})
		if err != nil {
			return err
		}
	}

	return nil
}
