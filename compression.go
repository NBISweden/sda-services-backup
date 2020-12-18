package main

import (
	"compress/zlib"
	"io"

	log "github.com/sirupsen/logrus"
)

func newCompressor(key []byte, w io.Writer) (io.WriteCloser, error) {

	zw := zlib.NewWriter(w)
	_, err := zlib.NewWriterLevel(zw, zlib.BestCompression)

	if err != nil {
		log.Error("Unable to set zlib writer level", err)
		return nil, err
	}
	return zw, nil
}

func newDecompressor(key []byte, r io.Reader) (io.ReadCloser, error) {

	zr, err := zlib.NewReader(r)
	if err != nil {
		log.Error("Unable to create zlib reader", err)
		return nil, err
	}
	return zr, err
}
