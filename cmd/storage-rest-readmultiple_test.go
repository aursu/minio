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

package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinylib/msgp/msgp"
)

// The storage REST API had a ReadMultiple call (POST .../rmpl). It joined
// the bucket, prefix and file names of its msgpack body to the drive path
// without checking them, so "../" in the bucket read any file the server
// process could read (CVE-2026-42600). Nothing in MinIO called it, and it
// is removed. The request is built by hand, as a node holding the internode
// token could send it.
func TestStorageRESTReadMultipleRemoved(t *testing.T) {
	client := newStorageRESTHTTPServerClient(t)

	const secret = "a file outside the drive"
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	bucket, err := filepath.Rel(client.endpoint.Path, outside)
	if err != nil {
		t.Fatal(err)
	}

	// ReadMultipleReq{Bucket: bucket, Files: {"secret"}, MaxSize: 1 MiB}
	body := msgp.AppendMapHeader(nil, 3)
	body = msgp.AppendString(body, "bk")
	body = msgp.AppendString(body, bucket)
	body = msgp.AppendString(body, "fl")
	body = msgp.AppendArrayHeader(body, 1)
	body = msgp.AppendString(body, "secret")
	body = msgp.AppendString(body, "ms")
	body = msgp.AppendInt64(body, 1<<20)

	resp, err := client.call(t.Context(), "/rmpl", nil, bytes.NewReader(body), int64(len(body)))
	if err == nil {
		data, _ := io.ReadAll(resp)
		resp.Close()
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("bucket %q read a file outside the drive", bucket)
		}
		t.Fatalf("the ReadMultiple call is still served: %q", data)
	}
}
