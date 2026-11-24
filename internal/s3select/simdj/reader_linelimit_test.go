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

package simdj

import (
	"io"
	"strings"
	"testing"

	"github.com/minio/minio/internal/s3select/json"
	"github.com/minio/minio/internal/s3select/linelimit"
	"github.com/minio/minio/internal/s3select/sql"
	"github.com/minio/simdjson-go"
)

// The bound itself is tested in package linelimit. These tests check that
// the simdjson reader passes its input through linelimit.Reader and reports
// ErrLineTooLong, and that valid input is unchanged.

// jsonLine returns a JSON object line of exactly n bytes, excluding newline.
func jsonLine(n int) string {
	const head, tail = `{"k":"`, `"}`
	return head + strings.Repeat("y", n-len(head)-len(tail)) + tail
}

func requireSIMD(t *testing.T) {
	t.Helper()
	if !simdjson.SupportedCPU() {
		t.Skip("simdjson not supported on this CPU; the PReader path covers this case")
	}
}

func readAll(t *testing.T, r *Reader) ([]string, error) {
	t.Helper()
	var out []string
	var rec sql.Record
	var err error
	for {
		rec, err = r.Read(rec)
		if err != nil {
			return out, err
		}
		v, verr := rec.(*Record).Get("k")
		if verr != nil {
			return out, verr
		}
		s, _ := v.ToString()
		out = append(out, s)
	}
}

func linesArgs() *json.ReaderArgs {
	return &json.ReaderArgs{ContentType: "lines"}
}

// TestReaderLineLimitBoundary checks limit and limit+1 for a line in the
// middle of a stream and for the last line without a trailing newline.
func TestReaderLineLimitBoundary(t *testing.T) {
	requireSIMD(t)
	cases := []struct {
		name    string
		input   string
		wantErr bool
		want    int
	}{
		{"limit-newline", `{"k":"a"}` + "\n" + jsonLine(linelimit.MaxLineSize) + "\n" + `{"k":"b"}` + "\n", false, 3},
		{"limit+1-newline", `{"k":"a"}` + "\n" + jsonLine(linelimit.MaxLineSize+1) + "\n" + `{"k":"b"}` + "\n", true, 0},
		{"limit-eof", `{"k":"a"}` + "\n" + jsonLine(linelimit.MaxLineSize), false, 2},
		{"limit+1-eof", `{"k":"a"}` + "\n" + jsonLine(linelimit.MaxLineSize+1), true, 0},
		{"limit-first", jsonLine(linelimit.MaxLineSize) + "\n", false, 1},
		{"limit+1-first", jsonLine(linelimit.MaxLineSize+1) + "\n", true, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewReader(io.NopCloser(strings.NewReader(c.input)), linesArgs())
			defer r.Close()
			recs, err := readAll(t, r)
			if c.wantErr {
				if err != linelimit.ErrLineTooLong {
					t.Fatalf("want ErrLineTooLong, got %v after %d records", err, len(recs))
				}
				for _, s := range recs {
					if len(s) > linelimit.MaxLineSize {
						t.Fatalf("record with %d-byte value returned despite limit", len(s))
					}
				}
				return
			}
			if err != io.EOF {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(recs) != c.want {
				t.Fatalf("want %d records, got %d", c.want, len(recs))
			}
		})
	}
}

// TestReaderValidInputUnchanged checks that ordinary JSON Lines keep
// working on the simdjson path.
func TestReaderValidInputUnchanged(t *testing.T) {
	requireSIMD(t)
	var sb strings.Builder
	const n = 200000 // > one 10 MiB ParseNDStream block in total
	for range n {
		sb.WriteString(`{"k":"` + strings.Repeat("v", 50) + `"}` + "\n")
	}
	r := NewReader(io.NopCloser(strings.NewReader(sb.String())), linesArgs())
	defer r.Close()
	recs, err := readAll(t, r)
	if err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != n {
		t.Fatalf("want %d records, got %d", n, len(recs))
	}
}
