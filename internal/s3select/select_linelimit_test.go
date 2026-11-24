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

package s3select

import (
	"bytes"
	"compress/gzip"
	"errors"
	"strings"
	"testing"
)

// newlineFreeGzip returns a GZIP object that decompresses into size bytes
// without a newline. size is kept small enough that the behaviour before the
// fix (buffering it whole) stays testable.
func newlineFreeGzip(t *testing.T, size int) []byte {
	t.Helper()
	var obj bytes.Buffer
	zw, err := gzip.NewWriterLevel(&obj, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte{'a'}, 1<<20)
	for range size / len(chunk) {
		if _, err := zw.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return obj.Bytes()
}

// TestCSVCompressedNoNewlineBounded reproduces CVE-2026-39414 /
// GHSA-h749-fxx7-pwpg: a small GZIP object that decompresses into a long
// run of bytes without any newline must fail with OverMaxRecordSize after a
// bounded amount of decompressed input, instead of being buffered whole.
func TestCSVCompressedNoNewlineBounded(t *testing.T) {
	obj := newlineFreeGzip(t, 32<<20)
	request := func(header string) string {
		return selectRequest("SELECT COUNT(*) FROM S3Object", "GZIP",
			`<CSV><FileHeaderInfo>`+header+`</FileHeaderInfo></CSV>`)
	}
	const maxProcessed = 4 << 20

	// Records are streamed, so the error comes as the stream's error event.
	t.Run("body", func(t *testing.T) {
		_, processed, err := evaluateProgress(t, request("NONE"), obj)
		wantErrorCode(t, err, "OverMaxRecordSize")
		if processed > maxProcessed {
			t.Fatalf("processed %d decompressed bytes, want <= %d", processed, maxProcessed)
		}
	})

	// The header line is read in Open, which must fail with the S3 error.
	t.Run("header", func(t *testing.T) {
		s3Select, err := NewS3Select(strings.NewReader(request("USE")))
		if err != nil {
			t.Fatal(err)
		}
		err = s3Select.Open(newBytesRSC(obj))
		_, processed := s3Select.getProgress()
		if err == nil {
			s3Select.Close()
			t.Fatalf("expected Open to fail (%d processed)", processed)
		}
		var serr SelectError
		if !errors.As(err, &serr) || serr.ErrorCode() != "OverMaxRecordSize" || serr.HTTPStatusCode() != 400 {
			t.Fatalf("want OverMaxRecordSize/400, got %v (%T)", err, err)
		}
		if processed > maxProcessed {
			t.Fatalf("processed %d decompressed bytes, want <= %d", processed, maxProcessed)
		}
	})
}

// TestJSONLinesCompressedNoNewlineBounded is the JSON Lines sibling of the
// CSV regression above, for both readers that Open can pick.
func TestJSONLinesCompressedNoNewlineBounded(t *testing.T) {
	obj := newlineFreeGzip(t, 64<<20)
	// The simdjson path reads a 10 MiB block before looking for a newline.
	const maxProcessed = 16 << 20
	forEachJSONLinesReader(t, func(t *testing.T) {
		request := selectRequest("SELECT COUNT(*) FROM S3Object", "GZIP", jsonLinesInput)
		_, processed, err := evaluateProgress(t, request, obj)
		wantErrorCode(t, err, "OverMaxRecordSize")
		if processed > maxProcessed {
			t.Fatalf("processed %d decompressed bytes, want <= %d", processed, maxProcessed)
		}
	})
}
