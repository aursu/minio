// Copyright (c) 2015-2026 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package linelimit bounds the input lines that the S3 Select readers buffer.
//
// The CSV and JSON Lines readers split their input at newlines. Reading up to
// the next newline without a bound reads a newline-free input into memory
// whole, so a small compressed object can exhaust the server's memory
// (CVE-2026-39414). Every reader goes through this package instead.
package linelimit

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// MaxLineSize is the maximum length in bytes of one input line, excluding
// its newline. It is the S3 Select maxCharsPerRecord limit of 1 MiB, which
// is also enforced on output records. Readers with another record delimiter
// convert it to '\n' before the input reaches this package.
//
// The advisory (GHSA-h749-fxx7-pwpg) describes a fix with a 128 KiB cap.
// That would reject lines the S3 contract accepts, so the limit stays at
// 1 MiB; memory per CSV or JSON split is then bounded by the split size,
// MaxLineSize and one bufio buffer.
const MaxLineSize = 1 << 20

// ErrLineTooLong reports an input line longer than MaxLineSize. It is an S3
// Select error (code OverMaxRecordSize, HTTP 400) and is returned unwrapped,
// so the s3select package and the API handler report it to the client as is.
var ErrLineTooLong error = lineTooLongError{}

type lineTooLongError struct{}

const lineTooLongMessage = "The length of a record in the input or result is greater than maxCharsPerRecord of 1 MB."

var errLineTooLongCause = errors.New("input line exceeds the maximum record size")

func (lineTooLongError) Error() string        { return lineTooLongMessage }
func (lineTooLongError) ErrorCode() string    { return "OverMaxRecordSize" }
func (lineTooLongError) ErrorMessage() string { return lineTooLongMessage }
func (lineTooLongError) HTTPStatusCode() int  { return 400 }
func (lineTooLongError) Cause() error         { return errLineTooLongCause }

// CompleteLine reads from buf up to and including the next newline and
// appends what it read to dst. dst must start at the beginning of a line,
// so the line being completed is the part of dst after its last newline.
//
// When that line grows beyond MaxLineSize, CompleteLine returns
// ErrLineTooLong and no data, having read at most MaxLineSize plus one
// buffer of buf. At the end of the input it returns dst with io.EOF.
func CompleteLine(buf *bufio.Reader, dst []byte) ([]byte, error) {
	return completeLine(buf, dst, MaxLineSize)
}

func completeLine(buf *bufio.Reader, dst []byte, limit int) ([]byte, error) {
	lineLen := len(dst) - (bytes.LastIndexByte(dst, '\n') + 1)
	for {
		in, err := buf.ReadSlice('\n')
		lineLen += len(in)
		if err == nil {
			lineLen-- // the newline is not part of the line
		}
		// Checked before appending, so dst never grows past the limit.
		if lineLen > limit {
			return nil, ErrLineTooLong
		}
		dst = append(dst, in...)
		if err != bufio.ErrBufferFull {
			return dst, err
		}
	}
}

// Reader passes its input through and fails with ErrLineTooLong once a line
// grows beyond MaxLineSize. It is for consumers that read up to a newline on
// their own and take no bound, such as the simdjson-go stream parser.
type Reader struct {
	r     io.Reader
	limit int
	run   int // length of the current line so far
	err   error
}

// NewReader returns a Reader that bounds the lines of r to MaxLineSize.
func NewReader(r io.Reader) *Reader {
	return &Reader{r: r, limit: MaxLineSize}
}

// Read reads from the underlying reader. In the read where a line passes
// the limit, it returns the input up to the limit together with
// ErrLineTooLong, and every later call returns ErrLineTooLong without
// reading.
func (l *Reader) Read(p []byte) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	n, err := l.r.Read(p)
	for off := 0; off < n; {
		end := n
		i := bytes.IndexByte(p[off:n], '\n')
		if i >= 0 {
			end = off + i
		}
		if l.run+end-off > l.limit {
			l.err = ErrLineTooLong
			return off + l.limit - l.run, l.err
		}
		if i < 0 {
			l.run += n - off
			break
		}
		l.run = 0
		off = end + 1
	}
	return n, err
}
