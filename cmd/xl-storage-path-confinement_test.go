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
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// The storage RPCs take volumes and paths from the request body, which the
// request validity middleware does not check, and xlStorage joined them
// to the drive path with pathJoin, which resolves ".." without confining
// it. A holder of the internode token could therefore read, write, rename
// and delete files outside the drive. These calls go through the storage
// REST client, so they reach the remote drive over the grid and HTTP
// routes a node uses. Each one points either its volume or its path at a
// directory beside the drive.
func TestStorageRESTPathConfinement(t *testing.T) {
	client := newStorageRESTHTTPServerClient(t)
	ctx := t.Context()

	const secret = "a file outside the drive"
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	// From the volume "foo" to outside, e.g. "../../003".
	rel, err := filepath.Rel(filepath.Join(client.endpoint.Path, "foo"), outside)
	if err != nil {
		t.Fatal(err)
	}
	rel = filepath.ToSlash(rel)

	// An object written inside the drive; its xl.meta is copied outside, so
	// that the metadata reads below have something to find there.
	fi := newFileInfo("obj", 1, 1)
	fi.VersionID = mustGetUUID()
	fi.ModTime = UTCNow()
	if err := client.WriteMetadata(ctx, "", "foo", "obj", fi); err != nil {
		t.Fatal(err)
	}
	xlMeta, err := os.ReadFile(filepath.Join(client.endpoint.Path, "foo", "obj", xlStorageFormatFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "obj"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "obj", xlStorageFormatFile), xlMeta, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := client.WriteAll(ctx, "foo", "inside", []byte("inside")); err != nil {
		t.Fatal(err)
	}

	// Each target is reached once through the volume and once through the
	// path.
	type target struct{ volume, prefix string }
	targets := map[string]target{
		"volume": {"foo/" + rel, ""},
		"path":   {"foo", rel + "/"},
	}
	for name, tg := range targets {
		t.Run(name, func(t *testing.T) {
			v, p := tg.volume, tg.prefix
			calls := []struct {
				name string
				call func() error
			}{
				{"ReadAll", func() error {
					b, err := client.ReadAll(ctx, v, p+"secret")
					if bytes.Contains(b, []byte(secret)) {
						t.Errorf("ReadAll read the file outside the drive")
					}
					return err
				}},
				{"ReadFileStream", func() error {
					rc, err := client.ReadFileStream(ctx, v, p+"secret", 0, int64(len(secret)))
					if err == nil {
						b, _ := io.ReadAll(rc)
						rc.Close()
						if bytes.Contains(b, []byte(secret)) {
							t.Errorf("ReadFileStream read the file outside the drive")
						}
					}
					return err
				}},
				{"StatInfoFile", func() error { _, err := client.StatInfoFile(ctx, v, p+"secret", false); return err }},
				{"ListDir", func() error { _, err := client.ListDir(ctx, "", v, p, -1); return err }},
				{"ReadXL", func() error { _, err := client.ReadXL(ctx, v, p+"obj", false); return err }},
				{"ReadVersion", func() error {
					_, err := client.ReadVersion(ctx, "", v, p+"obj", "", ReadOptions{})
					return err
				}},
				{"WriteAll", func() error { return client.WriteAll(ctx, v, p+"written", []byte("x")) }},
				{"WriteMetadata", func() error { return client.WriteMetadata(ctx, "", v, p+"planted", fi) }},
				{"CreateFile", func() error {
					return client.CreateFile(ctx, "", v, p+"created", 1, bytes.NewReader([]byte("x")))
				}},
				{"AppendFile", func() error { return client.AppendFile(ctx, v, p+"appended", []byte("x")) }},
				{"RenameFile into", func() error { return client.RenameFile(ctx, "foo", "inside", v, p+"moved") }},
				{"RenameFile out of", func() error { return client.RenameFile(ctx, v, p+"secret", "foo", "taken") }},
				{"Delete", func() error { return client.Delete(ctx, v, p+"secret", DeleteOptions{}) }},
				{"DeleteVersion", func() error { return client.DeleteVersion(ctx, v, p+"obj", fi, false, DeleteOptions{}) }},
			}
			for _, c := range calls {
				if err := c.call(); err == nil {
					t.Errorf("%s: no error", c.name)
				}
			}
		})
	}

	// Nothing outside the drive changed.
	if b, err := os.ReadFile(filepath.Join(outside, "secret")); err != nil || string(b) != secret {
		t.Errorf("the file outside the drive changed: %q, %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(outside, "obj", xlStorageFormatFile)); err != nil || !bytes.Equal(b, xlMeta) {
		t.Errorf("the xl.meta outside the drive changed: %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "secret" && e.Name() != "obj" {
			t.Errorf("a file was created outside the drive: %s", e.Name())
		}
	}
}

// Secondary paths from the request, and the paths of the calls the test
// above does not make, are confined too: a data directory, the old data
// directory of a delete, the skip-parent of a part rename, the base
// directory of a walk, the parts of ReadParts, and the bucket the scanner
// is asked to scan.
func TestXLStoragePathArguments(t *testing.T) {
	storage, _, err := newXLStorageTestSetup(t)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err := storage.MakeVol(ctx, "foo"); err != nil {
		t.Fatal(err)
	}
	bad := "../../../etc"

	fi := newFileInfo("obj", 1, 1)
	fi.DataDir = bad
	tests := []struct {
		name string
		err  error
		call func() error
	}{
		{"getVolDir", errVolumeAccessDenied, func() error { _, err := storage.StatVol(ctx, "foo/"+bad); return err }},
		{"MakeVol", errVolumeAccessDenied, func() error { return storage.MakeVol(ctx, "foo/"+bad+"/new") }},
		{"volume /", errVolumeAccessDenied, func() error { _, err := storage.StatVol(ctx, "/"); return err }},
		{"volume ///", errVolumeAccessDenied, func() error { _, err := storage.ListDir(ctx, "", "///", "", -1); return err }},
		{"volume \\", errVolumeAccessDenied, func() error { return storage.DeleteVol(ctx, `\\`, true) }},
		{"CheckParts DataDir", errFileAccessDenied, func() error { _, err := storage.CheckParts(ctx, "foo", "obj", fi); return err }},
		{"VerifyFile DataDir", errFileAccessDenied, func() error { _, err := storage.VerifyFile(ctx, "foo", "obj", fi); return err }},
		{"RenameData DataDir", errFileAccessDenied, func() error {
			_, err := storage.RenameData(ctx, "foo", "obj", fi, "foo", "obj2", RenameOptions{})
			return err
		}},
		{"DeleteVersion OldDataDir", errFileAccessDenied, func() error {
			return storage.DeleteVersion(ctx, "foo", "obj", newFileInfo("obj", 1, 1), false, DeleteOptions{OldDataDir: bad})
		}},
		{"RenamePart skipParent", errFileAccessDenied, func() error {
			return storage.RenamePart(ctx, "foo", "a", "foo", "b", nil, bad)
		}},
		{"ReadParts", errFileAccessDenied, func() error { _, err := storage.ReadParts(ctx, "foo", bad+"/part.1.meta"); return err }},
		{"DeleteBulk", errFileAccessDenied, func() error { return storage.DeleteBulk(ctx, "foo", "ok", bad) }},
		{"DeleteVersions", errFileAccessDenied, func() error {
			return storage.DeleteVersions(ctx, "foo", []FileInfoVersions{{Name: bad, Versions: []FileInfo{newFileInfo(bad, 1, 1)}}}, DeleteOptions{})[0]
		}},
		{"UpdateMetadata", errFileAccessDenied, func() error {
			return storage.UpdateMetadata(ctx, "foo", bad, newFileInfo("obj", 1, 1), UpdateMetadataOpts{})
		}},
		{"CleanAbandonedData", errFileAccessDenied, func() error { return storage.CleanAbandonedData(ctx, "foo", bad) }},
		{"WalkDir BaseDir", errFileAccessDenied, func() error {
			return storage.WalkDir(ctx, WalkDirOptions{Bucket: "foo", BaseDir: bad + "/"}, io.Discard)
		}},
		{"NSScanner bucket", errVolumeAccessDenied, func() error {
			var cache dataUsageCache
			cache.Info.Name = bad
			updates := make(chan dataUsageEntry, 100)
			_, err := storage.NSScanner(ctx, cache, updates, 0, func() bool { return false })
			return err
		}},
	}
	for _, tc := range tests {
		if err := tc.call(); !errors.Is(err, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.err)
		}
	}

	// Names a client can use still work: segments that only start or end
	// with a dot, and the multi-segment metadata volumes.
	for _, p := range []string{".hidden", "a/.b/c..", "..d/e"} {
		if err := storage.WriteAll(ctx, "foo", p, []byte("x")); err != nil {
			t.Errorf("WriteAll %q: %v", p, err)
		}
	}
	if err := storage.WriteAll(ctx, minioMetaTmpBucket, "confinement-check", []byte("x")); err != nil {
		t.Errorf("WriteAll in %s: %v", minioMetaTmpBucket, err)
	}
}
