package content

import (
	"archive/tar"
	"fmt"
	"io"

	"github.com/containerd/containerd/content"
)

// NewUntarWriter wrap a writer with an untar, so that the stream is untarred
//
// By default, it calculates the hash when writing. If the option `skipHash` is true,
// it will skip doing the hash. Skipping the hash is intended to be used only
// if you are confident about the validity of the data being passed to the writer,
// and wish to save on the hashing time.
func NewUntarWriter(writer content.Writer, opts ...WriterOpt) content.Writer {
	// process opts for default
	wOpts := DefaultWriterOpts()
	for _, opt := range opts {
		if err := opt(&wOpts); err != nil {
			return nil
		}
	}

	return NewPassthroughWriter(writer, func(r io.Reader, w io.Writer, done chan<- error) {
		tr := tar.NewReader(r)
		var err error
		for {
			_, err := tr.Next()
			if err == io.EOF {
				// clear the error, since we do not pass an io.EOF
				err = nil
				break // End of archive
			}
			if err != nil {
				// pass the error on
				err = fmt.Errorf("UntarWriter tar file header read error: %v", err)
				break
			}
			// write out the untarred data
			// we can handle io.EOF, just go to the next file
			// any other errors should stop and get reported
			b := make([]byte, wOpts.Blocksize, wOpts.Blocksize)
			for {
				var n int
				n, err = tr.Read(b)
				if err != nil && err != io.EOF {
					err = fmt.Errorf("UntarWriter file data read error: %v\n", err)
					break
				}
				l := n
				if n > len(b) {
					l = len(b)
				}
				if _, err2 := w.Write(b[:l]); err2 != nil {
					err = fmt.Errorf("UntarWriter error writing to underlying writer: %v", err2)
					break
				}
				if err == io.EOF {
					// go to the next file
					break
				}
			}
			// did we break with a non-nil and non-EOF error?
			if err != nil && err != io.EOF {
				break
			}
		}
		done <- err
	}, opts...)
}

// NewUntarWriterByName wrap multiple writers with an untar, so that the stream is untarred and passed
// to the appropriate writer, based on the filename. If a filename is not found, it will not pass it
// to any writer. The filename "" will handle any stream that does not have a specific filename; use
// it for the default of a single file in a tar stream.
func NewUntarWriterByName(writers map[string]content.Writer, opts ...WriterOpt) content.Writer {
	// process opts for default
	wOpts := DefaultWriterOpts()
	for _, opt := range opts {
		if err := opt(&wOpts); err != nil {
			return nil
		}
	}

	// construct an array of content.Writer
	nameToIndex := map[string]int{}
	var writerSlice []content.Writer
	for name, writer := range writers {
		writerSlice = append(writerSlice, writer)
		nameToIndex[name] = len(writerSlice) - 1
	}
	// need a PassthroughMultiWriter here
	return NewPassthroughMultiWriter(writerSlice, func(r io.Reader, ws []io.Writer, done chan<- error) {
		tr := tar.NewReader(r)
		var err error
		for {
			header, err := tr.Next()
			if err == io.EOF {
				// clear the error, since we do not pass an io.EOF
				err = nil
				break // End of archive
			}
			if err != nil {
				// pass the error on
				err = fmt.Errorf("UntarWriter tar file header read error: %v", err)
				break
			}
			// get the filename
			filename := header.Name
			index, ok := nameToIndex[filename]
			if !ok {
				index, ok = nameToIndex[""]
				if !ok {
					// we did not find this file or the wildcard, so do not process this file
					continue
				}
			}

			// write out the untarred data
			// we can handle io.EOF, just go to the next file
			// any other errors should stop and get reported
			b := make([]byte, wOpts.Blocksize, wOpts.Blocksize)
			for {
				var n int
				n, err = tr.Read(b)
				if err != nil && err != io.EOF {
					err = fmt.Errorf("UntarWriter file data read error: %v\n", err)
					break
				}
				l := n
				if n > len(b) {
					l = len(b)
				}
				if _, err2 := ws[index].Write(b[:l]); err2 != nil {
					err = fmt.Errorf("UntarWriter error writing to underlying writer at index %d for name '%s': %v", index, filename, err2)
					break
				}
				if err == io.EOF {
					// go to the next file
					break
				}
			}
			// did we break with a non-nil and non-EOF error?
			if err != nil && err != io.EOF {
				break
			}
		}
		done <- err
	}, opts...)
}
