package rpc

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// ReadLines reads JSON Lines records from r and hands each one to fn without
// its terminator. Records are split on LF only; one trailing CR is removed so
// CRLF input works; a final record without a terminator is delivered at end of
// stream; an empty record is delivered as an empty slice. No byte inside a
// record ends it, Unicode line separators (U+2028, U+2029) included, so a JSON
// string that contains one stays whole. A record is bounded only by memory.
//
// The slice handed to fn is owned by the callee. A non-nil error from fn stops
// the read and is returned; a read error other than io.EOF is returned as is.
func ReadLines(r io.Reader, fn func(line []byte) error) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if deliverErr := fn(trimRecord(line)); deliverErr != nil {
				return deliverErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// trimRecord removes the LF terminator and one CR before it.
func trimRecord(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte{'\n'})
	return bytes.TrimSuffix(line, []byte{'\r'})
}
