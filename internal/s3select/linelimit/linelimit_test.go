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

package linelimit

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// The readers split their input into blocks of splitSize bytes and read it
// through a bufio.Reader of twice that size (csvSplitSize, jsonSplitSize).
const (
	splitSize = 128 << 10
	bufSize   = 2 * splitSize
)

// countingByteReader yields limit copies of b without any newline and
// records how many bytes it handed out, so the tests can show that reading
// stops once a line passes the limit.
type countingByteReader struct {
	b     byte
	n     int64
	limit int64
}

func (c *countingByteReader) Read(p []byte) (int, error) {
	if c.n >= c.limit {
		return 0, io.EOF
	}
	if rem := c.limit - c.n; int64(len(p)) > rem {
		p = p[:rem]
	}
	for i := range p {
		p[i] = c.b
	}
	c.n += int64(len(p))
	return len(p), nil
}

// TestContract pins the documented limit and the S3 error it maps to, so
// neither can drift silently.
func TestContract(t *testing.T) {
	if MaxLineSize != 1<<20 {
		t.Fatalf("MaxLineSize = %d, want 1 MiB (S3 Select maxCharsPerRecord)", MaxLineSize)
	}
	se, ok := ErrLineTooLong.(interface {
		Cause() error
		ErrorCode() string
		ErrorMessage() string
		HTTPStatusCode() int
	})
	if !ok {
		t.Fatal("ErrLineTooLong does not implement s3select.SelectError")
	}
	if se.ErrorCode() != "OverMaxRecordSize" || se.HTTPStatusCode() != 400 || se.Cause() == nil ||
		!strings.Contains(se.ErrorMessage(), "maxCharsPerRecord") {
		t.Fatalf("unexpected error contract: %q %d %q %v", se.ErrorCode(), se.HTTPStatusCode(), se.ErrorMessage(), se.Cause())
	}
}

// split mirrors a reader's nextSplit: read skip bytes as the block, then
// complete the line the block ends in.
func split(src io.Reader, skip int) ([]byte, error) {
	buf := bufio.NewReaderSize(src, bufSize)
	dst := make([]byte, skip)
	if _, err := io.ReadFull(buf, dst); err != nil {
		return nil, err
	}
	return CompleteLine(buf, dst)
}

// TestCompleteLineBoundary checks the exact boundary for a line at the read
// position (skip 0, the CSV header), for a line that starts inside the block
// and for a block that ends on a newline.
func TestCompleteLineBoundary(t *testing.T) {
	const prefix = "short,line\n"
	y := func(n int) string { return strings.Repeat("y", n) }
	cases := []struct {
		name    string
		skip    int
		input   string
		wantErr bool
		wantLen int
	}{
		{"skip0-limit-newline", 0, y(MaxLineSize) + "\nnext\n", false, MaxLineSize + 1},
		{"skip0-limit+1-newline", 0, y(MaxLineSize+1) + "\nnext\n", true, 0},
		{"skip0-limit-eof", 0, y(MaxLineSize), false, MaxLineSize},
		{"skip0-limit+1-eof", 0, y(MaxLineSize + 1), true, 0},
		{"block-straddle-limit", splitSize, prefix + y(MaxLineSize) + "\nnext\n", false, len(prefix) + MaxLineSize + 1},
		{"block-straddle-limit+1", splitSize, prefix + y(MaxLineSize+1) + "\nnext\n", true, 0},
		{"block-from-start-limit", splitSize, y(MaxLineSize) + "\nnext\n", false, MaxLineSize + 1},
		{"block-from-start-limit+1", splitSize, y(MaxLineSize+1) + "\nnext\n", true, 0},
		{"block-ends-on-newline-limit", splitSize, y(splitSize-1) + "\n" + y(MaxLineSize) + "\n", false, splitSize + MaxLineSize + 1},
		{"block-ends-on-newline-limit+1", splitSize, y(splitSize-1) + "\n" + y(MaxLineSize+1) + "\n", true, 0},
		{"crlf-counts-cr", 0, y(MaxLineSize) + "\r\n", true, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := split(strings.NewReader(c.input), c.skip)
			if c.wantErr {
				if err != ErrLineTooLong || got != nil {
					t.Fatalf("want ErrLineTooLong and no data, got err=%v, %d bytes", err, len(got))
				}
				return
			}
			if err != nil && err != io.EOF {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != c.wantLen {
				t.Fatalf("want %d bytes, got %d", c.wantLen, len(got))
			}
		})
	}
}

// TestCompleteLineNoNewlineBounded feeds 64 MiB without a newline: the read
// must stop after the limit plus one buffer, not at EOF.
func TestCompleteLineNoNewlineBounded(t *testing.T) {
	for _, skip := range []int{0, splitSize} {
		src := &countingByteReader{b: 'a', limit: 64 << 20}
		got, err := split(src, skip)
		if err != ErrLineTooLong || got != nil {
			t.Fatalf("skip=%d: want ErrLineTooLong and no data, got err=%v, %d bytes", skip, err, len(got))
		}
		if maxRead := int64(skip + MaxLineSize + 2*bufSize); src.n > maxRead {
			t.Fatalf("skip=%d: consumed %d bytes of input, want <= %d", skip, src.n, maxRead)
		}
	}
}

// TestReader runs the wrapper with a small limit, with whole and with
// one-byte reads of the underlying input.
func TestReader(t *testing.T) {
	const limit = 16
	y := func(n int) string { return strings.Repeat("y", n) }
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", false},
		{"limit-newline", y(limit) + "\nnext\n", false},
		{"limit+1-newline", y(limit+1) + "\nnext\n", true},
		{"limit-eof", "a\n" + y(limit), false},
		{"limit+1-eof", "a\n" + y(limit+1), true},
		{"many-limit-lines", strings.Repeat(y(limit)+"\n", 10), false},
		{"empty-lines", "\n\n\n", false},
		{"crlf-counts-cr", y(limit) + "\r\n", true},
	}
	for _, c := range cases {
		for _, oneByte := range []bool{false, true} {
			name := c.name
			if oneByte {
				name += "/onebyte"
			}
			t.Run(name, func(t *testing.T) {
				var src io.Reader = strings.NewReader(c.input)
				if oneByte {
					src = iotest.OneByteReader(src)
				}
				got, err := io.ReadAll(&Reader{r: src, limit: limit})
				if !c.wantErr {
					if err != nil || string(got) != c.input {
						t.Fatalf("want the input back unchanged, got err=%v, %q", err, got)
					}
					return
				}
				if err != ErrLineTooLong {
					t.Fatalf("want ErrLineTooLong, got %v", err)
				}
				// The input is handed out up to the limit and no further.
				if !strings.HasPrefix(c.input, string(got)) {
					t.Fatalf("returned data is not a prefix of the input: %q", got)
				}
				if i := bytes.LastIndexByte(got, '\n'); len(got)-(i+1) != limit {
					t.Fatalf("returned %d bytes of the long line, want exactly %d", len(got)-(i+1), limit)
				}
			})
		}
	}
}

// TestReaderNoNewlineBounded checks with the real limit that the wrapper
// stops pulling input after the limit plus one read and that its error is
// sticky.
func TestReaderNoNewlineBounded(t *testing.T) {
	src := &countingByteReader{b: 'a', limit: 64 << 20}
	lr := NewReader(src)
	buf := make([]byte, 32<<10)
	var total int
	var err error
	for err == nil {
		var n int
		n, err = lr.Read(buf)
		total += n
	}
	if err != ErrLineTooLong {
		t.Fatalf("want ErrLineTooLong, got %v", err)
	}
	if total != MaxLineSize {
		t.Fatalf("returned %d bytes of the line, want exactly %d", total, MaxLineSize)
	}
	if src.n > int64(MaxLineSize+len(buf)) {
		t.Fatalf("consumed %d bytes of input", src.n)
	}
	consumed := src.n
	if n, err := lr.Read(buf); n != 0 || err != ErrLineTooLong || src.n != consumed {
		t.Fatalf("error not sticky: n=%d err=%v consumed %d -> %d", n, err, consumed, src.n)
	}
}

// benchInput is 64 MiB of 100-byte lines, a typical CSV or JSON Lines shape.
func benchInput() []byte {
	return bytes.Repeat([]byte(strings.Repeat("x", 99)+"\n"), 64<<20/100)
}

// BenchmarkReader measures the cost of the line scan that the simdjson path
// adds in front of the parser.
func BenchmarkReader(b *testing.B) {
	in := benchInput()
	buf := make([]byte, 32<<10)
	b.SetBytes(int64(len(in)))
	b.ResetTimer()
	for range b.N {
		r := NewReader(bytes.NewReader(in))
		for {
			if _, err := r.Read(buf); err != nil {
				break
			}
		}
	}
}

// BenchmarkCompleteLine measures splitting into blocks as the CSV and JSON
// Lines readers do.
func BenchmarkCompleteLine(b *testing.B) {
	in := benchInput()
	dst := make([]byte, 0, splitSize+MaxLineSize)
	b.SetBytes(int64(len(in)))
	b.ResetTimer()
	for range b.N {
		buf := bufio.NewReaderSize(bytes.NewReader(in), bufSize)
		for {
			d := dst[:splitSize]
			n, err := io.ReadFull(buf, d)
			if err != nil {
				break
			}
			if _, err := CompleteLine(buf, d[:n]); err != nil {
				break
			}
		}
	}
}
