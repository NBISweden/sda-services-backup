package main

import (
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	log "github.com/sirupsen/logrus"
)

var S3Port int

func TestMain(m *testing.M) {
	// uses a sensible default on windows (tcp/http) and linux/osx (socket)
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not construct pool: %s", err)
	}

	// uses pool to try to connect to Docker
	err = pool.Client.Ping()
	if err != nil {
		log.Fatalf("Could not connect to Docker: %s", err)
	}

	minio, err := pool.RunWithOptions(&dockertest.RunOptions{
		Name:       "s3",
		Repository: "minio/minio",
		Tag:        "RELEASE.2023-05-18T00-05-36Z",
		Cmd:        []string{"server", "/data", "--console-address", ":9001"},
		Env: []string{
			"MINIO_ROOT_USER=access",
			"MINIO_ROOT_PASSWORD=secretKey",
			"MINIO_SERVER_URL=http://127.0.0.1:9000",
		},
	}, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
	})
	if err != nil {
		log.Panicf("Could not start resource: %s", err)
	}

	s3HostAndPort := minio.GetHostPort("9000/tcp")
	S3Port, _ = strconv.Atoi(minio.GetPort("9000/tcp"))

	client := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "http://"+s3HostAndPort+"/minio/health/live", http.NoBody)
	if err != nil {
		log.Panic(err)
	}

	// exponential backoff-retry, because the application in the container might not be ready to accept connections yet
	if err := pool.Retry(func() error {
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		res.Body.Close()

		return nil
	}); err != nil {
		if err := pool.Purge(minio); err != nil {
			log.Panicf("Could not purge oidc resource: %s", err)
		}
		log.Panicf("Could not connect to oidc: %s", err)
	}

	log.Println("starting tests")
	code := m.Run()

	log.Println("tests completed")
	if err := pool.Purge(minio); err != nil {
		log.Fatalf("Could not purge resource: %s", err)
	}

	os.Exit(code)
}
