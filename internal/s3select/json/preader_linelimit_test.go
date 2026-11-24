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

package json

import (
	"io"
	"strings"
	"testing"

	"github.com/minio/minio/internal/s3select/linelimit"
	"github.com/minio/minio/internal/s3select/sql"
)

// The bound itself (limit, limit+1, lines across blocks, bytes consumed) is
// tested in package linelimit. These tests check that the JSON Lines reader
// is wired to it and that valid input and DOCUMENT mode are unchanged.

// jsonLine returns a JSON object line of exactly n bytes, excluding newline.
func jsonLine(n int) string {
	const head, tail = `{"k":"`, `"}`
	return head + strings.Repeat("y", n-len(head)-len(tail)) + tail
}

func readAllPReader(t *testing.T, r *PReader) ([]string, error) {
	t.Helper()
	var out []string
	var rec sql.Record
	var err error
	for {
		rec, err = r.Read(rec)
		if err != nil {
			return out, err
		}
		out = append(out, recordK(rec.(*Record)))
	}
}

// recordK returns the string value of key "k" of a decoded record.
// (*Record).Get is not implemented in this package; inspect KVS directly.
func recordK(rec *Record) string {
	for _, kv := range rec.KVS {
		if kv.Key == "k" {
			s, _ := kv.Value.(string)
			return s
		}
	}
	return ""
}

// TestPReaderLineTooLong checks that a single over-limit line in an
// otherwise valid JSON Lines stream is rejected with OverMaxRecordSize.
func TestPReaderLineTooLong(t *testing.T) {
	var sb strings.Builder
	for range 20000 {
		sb.WriteString(`{"k":"a"}` + "\n")
	}
	sb.WriteString(jsonLine(linelimit.MaxLineSize+1) + "\n")
	sb.WriteString(`{"k":"b"}` + "\n")
	r := NewPReader(io.NopCloser(strings.NewReader(sb.String())), &ReaderArgs{ContentType: "lines", unmarshaled: true})
	defer r.Close()
	recs, err := readAllPReader(t, r)
	if err != linelimit.ErrLineTooLong {
		t.Fatalf("want ErrLineTooLong, got %v after %d records", err, len(recs))
	}
	for _, s := range recs {
		if len(s) > linelimit.MaxLineSize {
			t.Fatalf("record with %d-byte value returned despite limit", len(s))
		}
	}
}

// TestPReaderValidInputUnchanged covers JSON Lines inputs that must keep
// working, including a line that is exactly at the limit and objects that
// span lines.
func TestPReaderValidInputUnchanged(t *testing.T) {
	limit := jsonLine(linelimit.MaxLineSize)
	limitVal := limit[len(`{"k":"`) : len(limit)-len(`"}`)]
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"simple", "{\"k\":\"1\"}\n{\"k\":\"2\"}\n", []string{"1", "2"}},
		{"no-trailing-newline", "{\"k\":\"1\"}\n{\"k\":\"2\"}", []string{"1", "2"}},
		{"crlf", "{\"k\":\"1\"}\r\n{\"k\":\"2\"}\r\n", []string{"1", "2"}},
		{"multi-line-object", "{\"k\":\n\"1\"}\n{\"k\":\"2\"}\n", []string{"1", "2"}},
		{"limit-line", "{\"k\":\"a\"}\n" + limit + "\n{\"k\":\"b\"}\n", []string{"a", limitVal, "b"}},
		{"limit-line-eof", "{\"k\":\"a\"}\n" + limit, []string{"a", limitVal}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewPReader(io.NopCloser(strings.NewReader(c.input)), &ReaderArgs{ContentType: "lines", unmarshaled: true})
			defer r.Close()
			recs, err := readAllPReader(t, r)
			if err != io.EOF {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(recs) != len(c.want) {
				t.Fatalf("want %d records, got %d", len(c.want), len(recs))
			}
			for i := range recs {
				if recs[i] != c.want[i] {
					t.Fatalf("record %d mismatch: got %d bytes", i, len(recs[i]))
				}
			}
		})
	}
}

// TestReaderDocumentLongLineUnchanged guards that DOCUMENT mode keeps its
// existing 10 MiB document cap and is not subject to the per-line limit.
func TestReaderDocumentLongLineUnchanged(t *testing.T) {
	doc := jsonLine(2 * linelimit.MaxLineSize)
	r := NewReader(io.NopCloser(strings.NewReader(doc)), &ReaderArgs{ContentType: "document", unmarshaled: true})
	defer r.Close()
	rec, err := r.Read(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s := recordK(rec.(*Record)); len(s) != len(doc)-len(`{"k":""}`) {
		t.Fatalf("got %d-byte value", len(s))
	}
	if _, err := r.Read(rec); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
}
