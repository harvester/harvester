package backupstore

import (
	"compress/gzip"
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/longhorn/backupstore/util"
)

func getBlockPath(volumeName string) string {
	return filepath.Join(getVolumePath(volumeName), BLOCKS_DIRECTORY) + "/"
}

func getBlockFilePath(volumeName, checksum string) string {
	blockSubDirLayer1 := checksum[0:BLOCK_SEPARATE_LAYER1]
	blockSubDirLayer2 := checksum[BLOCK_SEPARATE_LAYER1:BLOCK_SEPARATE_LAYER2]
	path := filepath.Join(getBlockPath(volumeName), blockSubDirLayer1, blockSubDirLayer2)
	fileName := checksum + BLK_SUFFIX

	return filepath.Join(path, fileName)
}

// mergeErrorChannels will merge all error channels into a single error out channel.
// the error out channel will be closed once the ctx is done or all error channels are closed
// if there is an error on one of the incoming channels the error will be relayed.
func mergeErrorChannels(ctx context.Context, channels ...<-chan error) <-chan error {
	var wg sync.WaitGroup
	wg.Add(len(channels))

	out := make(chan error, len(channels))
	output := func(c <-chan error) {
		defer wg.Done()
		select {
		case err, ok := <-c:
			if ok {
				out <- err
			}
			return
		case <-ctx.Done():
			return
		}
	}

	for _, c := range channels {
		go output(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// DecompressAndVerifyWithFallback decompresses the given data and verifies the data integrity.
// If the decompression fails, it will try to decompress with the fallback method.
func DecompressAndVerifyWithFallback(bsDriver BackupStoreDriver, blkFile, decompression, checksum string) (io.Reader, error) {
	// Helper function to read block from backup store
	readBlock := func() (io.ReadCloser, error) {
		rc, err := bsDriver.Read(blkFile)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read block %v", blkFile)
		}
		return rc, nil
	}

	// First attempt to read and decompress/verify
	rc, err := readBlock()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	r, err := util.DecompressAndVerify(decompression, rc, checksum)
	if err == nil {
		return r, nil
	}

	// If there's an error, determine the alternative decompression method
	alternativeDecompression := ""
	if strings.Contains(err.Error(), gzip.ErrHeader.Error()) {
		alternativeDecompression = "lz4"
	} else if strings.Contains(err.Error(), "lz4: bad magic number") {
		alternativeDecompression = "gzip"
	}

	// Second attempt with alternative decompression, if applicable
	if alternativeDecompression != "" {
		retriedRc, err := readBlock()
		if err != nil {
			return nil, err
		}
		defer retriedRc.Close()

		r, err = util.DecompressAndVerify(alternativeDecompression, retriedRc, checksum)
		if err != nil {
			return nil, errors.Wrapf(err, "fallback decompression also failed for block %v", blkFile)
		}
		return r, nil
	}

	return nil, errors.Wrapf(err, "decompression verification failed for block %v", blkFile)
}
