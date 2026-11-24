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
	"strings"
	"testing"

	"github.com/klauspost/cpuid/v2"
	"github.com/minio/minio/internal/s3select/linelimit"
	"github.com/minio/simdjson-go"
)

// selectRequest builds a request for expression over input, given as the
// InputSerialization body, for example `<CSV></CSV>`.
func selectRequest(expression, compression, input string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<SelectObjectContentRequest>
    <Expression>` + expression + `</Expression>
    <ExpressionType>SQL</ExpressionType>
    <InputSerialization>
        <CompressionType>` + compression + `</CompressionType>
        ` + input + `
    </InputSerialization>
    <OutputSerialization>
        <CSV>
        </CSV>
    </OutputSerialization>
    <RequestProgress>
        <Enabled>FALSE</Enabled>
    </RequestProgress>
</SelectObjectContentRequest>`
}

const jsonLinesInput = `<JSON><Type>LINES</Type></JSON>`

// evaluate runs one request over object and decodes the event stream into
// the record payload and the terminal error event.
func evaluate(t *testing.T, request string, object []byte) ([]byte, error) {
	t.Helper()
	records, _, err := evaluateProgress(t, request, object)
	return records, err
}

// evaluateProgress is evaluate that also returns the number of decompressed
// input bytes the query processed.
func evaluateProgress(t *testing.T, request string, object []byte) ([]byte, int64, error) {
	t.Helper()
	s3Select, err := NewS3Select(strings.NewReader(request))
	if err != nil {
		t.Fatal(err)
	}
	if err := s3Select.Open(newBytesRSC(object)); err != nil {
		t.Fatal(err)
	}
	w := &testResponseWriter{}
	s3Select.Evaluate(w)
	_, processed := s3Select.getProgress()
	s3Select.Close()
	records, err := decodeEvents(t, w.response)
	return records, processed, err
}

// forEachJSONLinesReader runs f with each reader that Open can pick for JSON
// Lines: simdjson where the CPU supports it, and the portable json.PReader,
// forced by hiding AVX2 from simdjson.SupportedCPU.
func forEachJSONLinesReader(t *testing.T, f func(t *testing.T)) {
	t.Run("native", func(t *testing.T) {
		t.Logf("simdjson.SupportedCPU() = %v", simdjson.SupportedCPU())
		f(t)
	})
	t.Run("preader", func(t *testing.T) {
		if cpuid.CPU.Supports(cpuid.AVX2) {
			cpuid.CPU.Disable(cpuid.AVX2)
			defer cpuid.CPU.Enable(cpuid.AVX2)
		}
		if simdjson.SupportedCPU() {
			t.Fatal("could not force the portable JSON Lines reader")
		}
		f(t)
	})
}

// wantErrorCode checks the code of the terminal error event.
func wantErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want a %s error event, the stream ended without error", code)
	}
	if !strings.Contains(err.Error(), code) {
		t.Fatalf("want a %s error event, got %v", code, err)
	}
}

// TestEvaluateKeepsSelectErrorCode: a reader error that is an S3 Select
// error reaches the client with its own code; any other error is still
// reported as InternalError.
func TestEvaluateKeepsSelectErrorCode(t *testing.T) {
	t.Run("other-error", func(t *testing.T) {
		_, err := evaluate(t, selectRequest("SELECT CAST(_1 AS INT) FROM S3Object", "NONE", `<CSV></CSV>`),
			[]byte("1\n2\nx\n"))
		wantErrorCode(t, err, "InternalError")
	})
	t.Run("json-lines", func(t *testing.T) {
		forEachJSONLinesReader(t, func(t *testing.T) {
			_, err := evaluate(t, selectRequest("SELECT * FROM S3Object", "NONE", jsonLinesInput),
				[]byte("{\"a\":1}\n{bad}\n"))
			wantErrorCode(t, err, "JSONParsingError")
		})
	})
}

// TestEvaluateSendsRecordsBeforeError: the records read before an error are
// sent before the error event, including those still in the output queue.
func TestEvaluateSendsRecordsBeforeError(t *testing.T) {
	// The CSV reader splits at 128 KiB plus the rest of the line: with 8-byte
	// lines the first block holds 16384+1 records. The second block starts
	// with an over-limit line and fails. 16385 is not a multiple of the
	// 100-record output queue, so 85 records are still queued at the error.
	const first = 128<<10/8 + 1
	var in bytes.Buffer
	for range first {
		in.WriteString("1234567\n")
	}
	in.WriteString(strings.Repeat("y", linelimit.MaxLineSize+1) + "\n")

	records, err := evaluate(t, selectRequest("SELECT * FROM S3Object", "NONE", `<CSV></CSV>`), in.Bytes())
	wantErrorCode(t, err, "OverMaxRecordSize")
	if got := bytes.Count(records, []byte("1234567\n")); got != first {
		t.Fatalf("want %d records before the error, got %d", first, got)
	}
}

// TestEvaluateReportsReadErrorAfterLastBlock: a read error on the last block,
// here a GZIP checksum mismatch at the end of the object, fails the query
// for every reader instead of ending it as if the object were complete.
func TestEvaluateReportsReadErrorAfterLastBlock(t *testing.T) {
	corrupt := func(t *testing.T, body string) []byte {
		t.Helper()
		var obj bytes.Buffer
		zw := gzip.NewWriter(&obj)
		if _, err := zw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		b := obj.Bytes()
		b[len(b)-8] ^= 0xff // CRC-32 of the GZIP trailer
		return b
	}
	t.Run("csv", func(t *testing.T) {
		_, err := evaluate(t, selectRequest("SELECT COUNT(*) FROM S3Object", "GZIP", `<CSV></CSV>`),
			corrupt(t, "a,b\n1,2\n"))
		if err == nil {
			t.Fatal("corrupt GZIP object read as complete")
		}
	})
	t.Run("json-lines", func(t *testing.T) {
		forEachJSONLinesReader(t, func(t *testing.T) {
			_, err := evaluate(t, selectRequest("SELECT COUNT(*) FROM S3Object", "GZIP", jsonLinesInput),
				corrupt(t, "{\"a\":1}\n{\"a\":2}\n"))
			if err == nil {
				t.Fatal("corrupt GZIP object read as complete")
			}
		})
	})
}
