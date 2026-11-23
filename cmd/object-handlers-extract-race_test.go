// Copyright (c) 2026 aursu/minio contributors
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
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/minio/minio/internal/auth"
	xhttp "github.com/minio/minio/internal/http"
)

// Snowball extraction writes small entries from parallel goroutines, each of
// which checks the entry's permission. Run under -race (make test-race): the
// per-entry checks shared r.Form, the request's log info and the handler's
// error variable, and a denied entry wrote the response from its goroutine.
func TestPutObjectExtractParallelEntries(t *testing.T) {
	ExecObjectLayerAPITest(ExecObjectLayerAPITestArgs{t: t, objAPITest: testPutObjectExtractParallelEntries})
}

// extractTestArchive returns a tar of count small entries under each prefix.
func extractTestArchive(t *testing.T, count int, prefixes ...string) []byte {
	t.Helper()
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	for _, prefix := range prefixes {
		for i := range count {
			body := fmt.Appendf(nil, "%s entry %d\n", prefix, i)
			if err := tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("%s/e%02d.txt", prefix, i), Mode: 0o600, Size: int64(len(body))}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func testPutObjectExtractParallelEntries(obj ObjectLayer, instance, bucket string, router http.Handler, cred auth.Credentials, t *testing.T) {
	const entries = 16

	t.Run(instance+"/signed", func(t *testing.T) {
		archive := extractTestArchive(t, entries, "signed")
		req, err := newTestSignedRequestV4(http.MethodPut, "http://localhost/"+bucket+"/signed.tar", int64(len(archive)), bytes.NewReader(archive),
			cred.AccessKey, cred.SecretKey, map[string]string{xhttp.AmzSnowballExtract: "true"})
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		for i := range entries {
			if _, err := obj.GetObjectInfo(context.Background(), bucket, fmt.Sprintf("signed/e%02d.txt", i), ObjectOptions{}); err != nil {
				t.Fatalf("entry %d not written: %v", i, err)
			}
		}
	})

	t.Run(instance+"/anonymous-denied-prefix", func(t *testing.T) {
		policy := fmt.Appendf(nil, `{"Version":"2012-10-17","Statement":[`+
			`{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:PutObject"],"Resource":["arn:aws:s3:::%[1]s/*"]},`+
			`{"Effect":"Deny","Principal":{"AWS":["*"]},"Action":["s3:PutObject"],"Resource":["arn:aws:s3:::%[1]s/no/*"]}]}`, bucket)
		req, err := newTestSignedRequestV4(http.MethodPut, "http://localhost/"+bucket+"?policy", int64(len(policy)), bytes.NewReader(policy), cred.AccessKey, cred.SecretKey, nil)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("install bucket policy: status %d: %s", rec.Code, rec.Body.String())
		}

		archive := extractTestArchive(t, entries, "ok", "no")
		req, err = newTestRequest(http.MethodPut, "http://localhost/"+bucket+"/anonymous.tar", int64(len(archive)), bytes.NewReader(archive))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(xhttp.AmzSnowballExtract, "true")
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "<Code>AccessDenied</Code>") {
			t.Fatalf("status %d, want 403 AccessDenied: %s", rec.Code, rec.Body.String())
		}
		if n := strings.Count(rec.Body.String(), "<Error>"); n != 1 {
			t.Fatalf("response has %d error documents, want 1: %s", n, rec.Body.String())
		}
		for i := range entries {
			if _, err := obj.GetObjectInfo(context.Background(), bucket, fmt.Sprintf("no/e%02d.txt", i), ObjectOptions{}); err == nil {
				t.Fatalf("denied entry no/e%02d.txt was written", i)
			}
		}
	})
}
