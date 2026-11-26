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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/minio/minio/internal/auth"
	"github.com/minio/minio/internal/crypto"
)

func TestAPICompressedEncryptedRange(t *testing.T) {
	ExecExtendedObjectLayerAPITest(t, testAPICompressedEncryptedRange, []string{"PutObject", "GetObject"})
}

// A highly compressible object compresses into less than one 64 KiB
// encryption package. A range that starts exactly on an entry of the
// compression index (every 1 MiB) then starts on disk at offset 0, with
// nothing left to skip after decompression, but inside the decrypted
// package, at a compression block that has no stream identifier. The
// reader expected one and the read failed with an unexpected EOF. The
// object is written through the PUT handler, which compresses and, in the
// encrypted runs, encrypts it.
func testAPICompressedEncryptedRange(obj ObjectLayer, instanceType, bucketName string, apiRouter http.Handler,
	credentials auth.Credentials, t *testing.T,
) {
	const objectName = "range-check.txt"
	// More than compMinIndexSize, so that the compression index is written.
	data := bytes.Repeat([]byte("minio compressed and encrypted range line\n"), (10<<20)/42+1)[:10<<20]

	req, err := newTestSignedRequestV4(http.MethodPut, getPutObjectURL("", bucketName, objectName),
		int64(len(data)), bytes.NewReader(data), credentials.AccessKey, credentials.SecretKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: PUT: %d %s", instanceType, rec.Code, rec.Body.String())
	}

	oi, err := obj.GetObjectInfo(t.Context(), bucketName, objectName, ObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, encrypted := crypto.IsEncrypted(oi.UserDefined)
	compressed := oi.IsCompressed()
	if compressed && (len(oi.Parts) == 0 || len(oi.Parts[0].Index) == 0) {
		t.Fatalf("%s: the compressed object has no index", instanceType)
	}

	for _, start := range []int{0, 100, 1 << 20, 1<<20 + 1, 2 << 20, 8 << 20, 9<<20 - 1, len(data) - 1000} {
		end := min(start+999, len(data)-1)
		req, err := newTestSignedRequestV4(http.MethodGet, getGetObjectURL("", bucketName, objectName),
			0, nil, credentials.AccessKey, credentials.SecretKey, map[string]string{"Range": fmt.Sprintf("bytes=%d-%d", start, end)})
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		apiRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusPartialContent || !bytes.Equal(rec.Body.Bytes(), data[start:end+1]) {
			t.Errorf("%s (compressed %t, encrypted %t): bytes=%d-%d: status %d, %d bytes, want 206 and %d exact bytes",
				instanceType, compressed, encrypted, start, end, rec.Code, rec.Body.Len(), end-start+1)
		}
	}
}
