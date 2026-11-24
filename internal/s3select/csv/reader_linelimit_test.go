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

package csv

import (
	"io"
	"strings"
	"testing"

	"github.com/minio/minio/internal/s3select/linelimit"
	"github.com/minio/minio/internal/s3select/sql"
)

// The bound itself (limit, limit+1, lines across blocks, bytes consumed) is
// tested in package linelimit. These tests check that the CSV reader is
// wired to it: header and body, every record delimiter, valid input intact.

func testReaderArgs(header, recordDelimiter string) *ReaderArgs {
	return &ReaderArgs{
		FileHeaderInfo:       header,
		RecordDelimiter:      recordDelimiter,
		FieldDelimiter:       ",",
		QuoteCharacter:       defaultQuoteCharacter,
		QuoteEscapeCharacter: defaultQuoteEscapeCharacter,
		CommentCharacter:     defaultCommentCharacter,
		unmarshaled:          true,
	}
}

func readAll(t *testing.T, r *Reader) ([][]string, error) {
	t.Helper()
	var out [][]string
	var rec sql.Record
	var err error
	for {
		rec, err = r.Read(rec)
		if err != nil {
			return out, err
		}
		out = append(out, append([]string(nil), rec.(*Record).csvRecord...))
	}
}

// The header line is read in NewReader, so the error must come from there.
func TestReaderHeaderLineTooLong(t *testing.T) {
	input := strings.Repeat("h", linelimit.MaxLineSize+1) + "\n1,2\n"
	r, err := NewReader(io.NopCloser(strings.NewReader(input)), testReaderArgs(use, "\n"))
	if err != linelimit.ErrLineTooLong {
		if r != nil {
			r.Close()
		}
		t.Fatalf("want ErrLineTooLong, got %v", err)
	}
}

func TestReaderBodyLineTooLong(t *testing.T) {
	for _, delim := range []string{"\n", "\r\n", ";"} {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(delim, "\r", `\r`), "\n", `\n`), func(t *testing.T) {
			var sb strings.Builder
			for range 2000 {
				sb.WriteString("a,b" + delim)
			}
			sb.WriteString(strings.Repeat("y", linelimit.MaxLineSize+1) + delim)
			sb.WriteString("c,d" + delim)
			r, err := NewReader(io.NopCloser(strings.NewReader(sb.String())), testReaderArgs(none, delim))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			recs, err := readAll(t, r)
			if err != linelimit.ErrLineTooLong {
				t.Fatalf("want ErrLineTooLong, got %v after %d records", err, len(recs))
			}
			for _, rec := range recs {
				for _, f := range rec {
					if len(f) > linelimit.MaxLineSize {
						t.Fatalf("record with %d-byte field returned despite limit", len(f))
					}
				}
			}
		})
	}
}

// TestReaderValidInputUnchanged covers inputs that must keep working:
// quoted fields containing newlines, alternative record delimiters and a
// line that is exactly at the limit.
func TestReaderValidInputUnchanged(t *testing.T) {
	long := strings.Repeat("y", linelimit.MaxLineSize-2)
	cases := []struct {
		name   string
		delim  string
		header string
		input  string
		want   [][]string
	}{
		{"quoted-newline", "\n", none, "x,\"multi\nline\"\nlast,row\n", [][]string{{"x", "multi\nline"}, {"last", "row"}}},
		{"crlf", "\r\n", none, "1,2\r\n3,4\r\n", [][]string{{"1", "2"}, {"3", "4"}}},
		{"tab-delim", "\t", none, "1,2\t3,4\t", [][]string{{"1", "2"}, {"3", "4"}}},
		{"limit-line-lf", "\n", none, "a,b\n" + long + ",z\nc,d\n", [][]string{{"a", "b"}, {long, "z"}, {"c", "d"}}},
		{"limit-line-crlf", "\r\n", none, "a,b\r\n" + long + ",z\r\nc,d\r\n", [][]string{{"a", "b"}, {long, "z"}, {"c", "d"}}},
		{"limit-header", "\n", use, long + ",z\n1,2\n", [][]string{{"1", "2"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := NewReader(io.NopCloser(strings.NewReader(c.input)), testReaderArgs(c.header, c.delim))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			recs, err := readAll(t, r)
			if err != io.EOF {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(recs) != len(c.want) {
				t.Fatalf("want %d records, got %d", len(c.want), len(recs))
			}
			for i := range recs {
				if strings.Join(recs[i], "\x00") != strings.Join(c.want[i], "\x00") {
					t.Fatalf("record %d mismatch: got %q", i, recs[i])
				}
			}
			if c.header == use && (len(r.columnNames) != 2 || r.columnNames[0] != long) {
				t.Fatalf("header not parsed")
			}
		})
	}
}
