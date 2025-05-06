package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/neicnordic/crypt4gh/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type S3TestSuite struct {
	suite.Suite
	Conf           S3Config
	Passphrase     []byte
	PrivateKey     [32]byte
	PrivateKeyPath string
	PublicKey      [32]byte
	PublicKeyPath  string
}

func TestS3TestSuite(t *testing.T) {
	suite.Run(t, new(S3TestSuite))
}

func (suite *S3TestSuite) SetupSuite() {
	var err error

	suite.Conf = S3Config{
		"http://127.0.0.1",
		S3Port,
		"access",
		"secretKey",
		"bucket",
		"region",
		10,
		"",
		"",
	}

	suite.PublicKey, suite.PrivateKey, err = keys.GenerateKeyPair()
	if err != nil {
		suite.T().Log("failed to generate c4gh keypair")
		suite.T().FailNow()
	}
	tempPath, _ := os.MkdirTemp("", "test")
	suite.PublicKeyPath = tempPath + "/test.pub"
	pubKey, err := os.Create(suite.PublicKeyPath)
	if err != nil {
		suite.T().Log("failed to create pubkey file")
		suite.T().FailNow()
	}
	if err := keys.WriteCrypt4GHX25519PublicKey(pubKey, suite.PublicKey); err != nil {
		suite.T().Log("failed to write pubk key to file")
		suite.T().FailNow()
	}

	suite.PrivateKeyPath = tempPath + "/test.key"
	privateKey, err := os.Create(suite.PrivateKeyPath)
	if err != nil {
		suite.T().Log("failed to create private key file")
		suite.T().FailNow()
	}
	suite.Passphrase = []byte("passphrase")
	if err := keys.WriteCrypt4GHX25519PrivateKey(privateKey, suite.PrivateKey, suite.Passphrase); err != nil {
		suite.T().Log("failed to write pubk key to file")
		suite.T().FailNow()
	}

	data, _ := os.MkdirTemp("", "data")
	randomData := new(bytes.Buffer)
	_, err = io.Copy(randomData, io.LimitReader(rand.Reader, int64(16*1024*1024)))
	if err != nil {
		suite.T().Log("failed to write random data")
		suite.T().FailNow()
	}

	if err := os.WriteFile(data+"/file", randomData.Bytes(), 0600); err != nil {
		suite.T().Log("failed to generate data file")
		suite.T().FailNow()
	}

	sb, err := newS3Backend(suite.Conf)
	if err != nil {
		suite.T().Log("failed to generate c4gh keypair")
		suite.T().FailNow()
	}

	files := []string{
		"base.file",
		"foo/foo.file1",
		"foo/foo.file2",
		"foo/bar/foobar.file1",
		"foo/bar/foobar.file2",
	}
	for _, f := range files {
		fr, err := os.Open(data + "/file")
		if err != nil {
			suite.T().Log("failed to open data file")
			suite.T().FailNow()
		}

		_, err = sb.Uploader.Upload(&s3manager.UploadInput{
			Body:            fr,
			Bucket:          aws.String(sb.Bucket),
			Key:             aws.String(f),
			ContentEncoding: aws.String("application/octet-stream"),
		})
		if err != nil {
			_ = fr.Close()
			suite.T().Logf("failed to upload files to bucket, reason: %s", err.Error())
			suite.T().FailNow()
		}
		err = fr.Close()
		if err != nil {
			suite.T().Logf("failed to close file %s\n", fr.Name())
			suite.T().FailNow()
		}
	}
	err = os.RemoveAll(data)
	if err != nil {
		suite.T().Log("failed to remove all test files")
		suite.T().FailNow()
	}
}

func (suite *S3TestSuite) TeardownSuite() {
	err := os.RemoveAll(suite.PublicKeyPath)
	if err != nil {
		suite.T().Logf("failed to remove file %s\n", suite.PublicKeyPath)
		suite.T().FailNow()
	}
}

func (suite *S3TestSuite) TestNewBackend() {
	backend, err := newS3Backend(suite.Conf)
	assert.NoError(suite.T(), err, "Setup failed unexpectedly")
	assert.Equal(suite.T(), "bucket", backend.Bucket)

	badConf := suite.Conf
	badConf.Port = 1111
	_, err = newS3Backend(badConf)
	assert.ErrorContains(suite.T(), err, "connection refused")
}

func (suite *S3TestSuite) TestBackupAndRestoreS3BucketEncrypted() {
	srcConf := suite.Conf
	src, err := newS3Backend(srcConf)
	assert.NoError(suite.T(), err, "failed to create source backend")

	source, err := src.Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: &src.Bucket,
		Prefix: &src.PathPrefix,
	})
	if err != nil {
		suite.T().Error()
	}
	assert.Equal(suite.T(), 5, int(*source.KeyCount))

	dstConf := suite.Conf
	dstConf.Bucket = "dst"
	dst, err := newS3Backend(dstConf)
	if err != nil {
		suite.T().Logf("failed to create destination backend, reason :%s", err.Error())
		suite.T().FailNow()
	}

	assert.NoError(suite.T(), BackupS3BucketEncrypted(src, dst, suite.PublicKeyPath), "failed to sync bucket")

	backedup, err := dst.Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: &dst.Bucket,
		Prefix: &dst.PathPrefix,
	})
	if err != nil {
		suite.T().Error()
	}
	assert.Equal(suite.T(), 5, int(*backedup.KeyCount))

	b := 0
	for _, so := range source.Contents {
		for _, bo := range backedup.Contents {
			if fmt.Sprintf("%s.c4gh", *so.Key) == *bo.Key && *bo.Size > *so.Size {
				b++

				break
			}
		}
	}
	assert.Equal(suite.T(), 5, b, "not all objects backedup")

	// test restoring encrypted backups to a new bucket
	restConf := suite.Conf
	restConf.Bucket = "restored"
	restore, err := newS3Backend(restConf)
	assert.NoError(suite.T(), err, "failed to create restore backend")

	assert.NoError(suite.T(), RestoreEncryptedS3Bucket(dst, restore, string(suite.Passphrase), suite.PrivateKeyPath), "failed to restore bucket")

	restored, err := restore.Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: &restore.Bucket,
		Prefix: &restore.PathPrefix,
	})
	if err != nil {
		suite.T().Error()
	}
	assert.Equal(suite.T(), 5, int(*restored.KeyCount))

	r := 0
	for _, so := range source.Contents {
		for _, ro := range restored.Contents {
			if *ro.Key == *so.Key && *so.Size == *ro.Size {
				r++

				break
			}
		}
	}
	assert.Equal(suite.T(), 5, r, "not all objects restored")
}

func (suite *S3TestSuite) TestBackupAndRestoreS3BucketSubPathEncrypted() {
	srcConf := suite.Conf
	srcConf.PathPrefix = "foo/bar"
	src, err := newS3Backend(srcConf)
	assert.NoError(suite.T(), err, "failed to create source backend")

	source, err := src.Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: &src.Bucket,
		Prefix: &src.PathPrefix,
	})
	if err != nil {
		suite.T().Error()
	}
	assert.Equal(suite.T(), 2, int(*source.KeyCount))

	dstConf := suite.Conf
	dstConf.Bucket = "dst2"
	dst, err := newS3Backend(dstConf)
	if err != nil {
		suite.T().Logf("failed to create destination backend, reason :%s", err.Error())
		suite.T().FailNow()
	}
	assert.NoError(suite.T(), BackupS3BucketEncrypted(src, dst, suite.PublicKeyPath), "failed to sync bucket")

	backup, err := dst.Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: &dst.Bucket,
		Prefix: &dst.PathPrefix,
	})
	if err != nil {
		suite.T().Error()
	}
	assert.Equal(suite.T(), 2, int(*backup.KeyCount))

	b := 0
	for _, so := range source.Contents {
		for _, bo := range backup.Contents {
			if fmt.Sprintf("%s.c4gh", *so.Key) == *bo.Key && *bo.Size > *so.Size {
				b++

				break
			}
		}
	}
	assert.Equal(suite.T(), 2, b, "not all objects backedup")

	// test restoreing encrypted backups to a new bucket
	restConf := suite.Conf
	restConf.Bucket = "sub"
	restore, err := newS3Backend(restConf)
	assert.NoError(suite.T(), err, "failed to create restore backend")

	assert.NoError(suite.T(), RestoreEncryptedS3Bucket(dst, restore, string(suite.Passphrase), suite.PrivateKeyPath), "failed to restore bucket")

	restored, err := restore.Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: &restore.Bucket,
		Prefix: &restore.PathPrefix,
	})
	if err != nil {
		suite.T().Error()
	}
	assert.Equal(suite.T(), 2, int(*restored.KeyCount))

	r := 0
	for _, so := range source.Contents {
		for _, ro := range restored.Contents {
			if *ro.Key == *so.Key && *so.Size == *ro.Size {
				r++

				break
			}
		}
	}
	assert.Equal(suite.T(), 2, r, "not all objects restored")
}

func (suite *S3TestSuite) TestSyncS3Buckets() {
	srcConf := suite.Conf
	src, err := newS3Backend(srcConf)
	assert.NoError(suite.T(), err, "failed to create source backend")

	source, err := src.Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket:  &src.Bucket,
		MaxKeys: aws.Int64(100),
		Prefix:  &src.PathPrefix,
	})
	if err != nil {
		suite.T().Error()
	}
	assert.Equal(suite.T(), 5, int(*source.KeyCount))

	dstConf := suite.Conf
	dstConf.Bucket = "sync"
	dst, err := newS3Backend(dstConf)
	if err != nil {
		suite.T().Logf("failed to create destination backend, reason :%s", err.Error())
		suite.T().FailNow()
	}
	assert.NoError(suite.T(), SyncS3Buckets(src, dst), "failed to sync bucket")

	destination, err := dst.Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: &dst.Bucket,
		Prefix: &dst.PathPrefix,
	})
	if err != nil {
		suite.T().Error()
	}
	assert.Equal(suite.T(), 5, int(*destination.KeyCount))

	b := 0
	for _, so := range source.Contents {
		for _, do := range destination.Contents {
			if *so.Key == *do.Key && *do.Size == *so.Size {
				b++

				break
			}
		}
	}
	assert.Equal(suite.T(), 5, b, "not all objects synced")
}
