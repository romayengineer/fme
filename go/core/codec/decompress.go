package codec

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/romayengineer/fme/go/core/logging"
)

type DecompressorFunc func(data []byte) ([]byte, error)

// list of decompressors in order or most likely used
var Decompressors = []DecompressorFunc{
	DecompressZstd,
	DecompressGzip,
	DecompressBrotli,
}

func Decompress(data []byte) ([]byte, error) {

	encoding, err := DetectBestWithConfidence(data)
	if err == nil {
		logging.LogDebug0("Decompress", "encoding match", "encoding", encoding)
		return DecodeWithEncoding(data, encoding)
	}

	for _, decompressor := range Decompressors {
		newData, err := decompressor(data)

		if err != nil {
			continue
		}

		encoding, err := DetectBestWithConfidence(newData)
		if err == nil {
			logging.LogDebug0("Decompress", "encoding match", "encoding", encoding)
			return DecodeWithEncoding(newData, encoding)
		}
	}

	return nil, fmt.Errorf("data is not compressed")
}

func DecompressBrotli(data []byte) ([]byte, error) {
	reader := brotli.NewReader(bytes.NewReader(data))
	result, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error decompressing brotli: %w", err)
	}
	return result, nil
}

func DecompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("error creating gzip reader: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	result, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error decompressing gzip: %w", err)
	}
	return result, nil
}

func DecompressZstd(data []byte) ([]byte, error) {
	reader, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("error creating zstd reader: %w", err)
	}
	defer reader.Close()

	result, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error decompressing zstd: %w", err)
	}
	return result, nil
}
