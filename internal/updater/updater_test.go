package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func serve(body []byte, failAfter int64) Downloader {
	return func(_ context.Context, _ string, offset int64, w io.Writer) error {
		rest := body[offset:]
		if failAfter > 0 && int64(len(rest)) > failAfter {
			w.Write(rest[:failAfter])
			return errors.New("connection lost")
		}
		_, err := w.Write(rest)
		return err
	}
}

func TestFetchVerifiesResumesAndRejects(t *testing.T) {
	body := bytes.Repeat([]byte("installer"), 10_000)
	h := sha256.Sum256(body)
	sum := hex.EncodeToString(h[:])
	dir := t.TempDir()

	// The connection drops part way: the partial file is kept...
	if _, err := Fetch(context.Background(), serve(body, 30_000), dir, "zonn-node_0.3.0_amd64.deb", int64(len(body)), sum, nil); err == nil {
		t.Fatal("a broken download succeeded")
	}
	if info, err := os.Stat(filepath.Join(dir, "zonn-node_0.3.0_amd64.deb.part")); err != nil || info.Size() != 30_000 {
		t.Fatalf("partial file: %v", err)
	}
	// ...and the next attempt resumes from it.
	var last int64
	path, err := Fetch(context.Background(), serve(body, 0), dir, "zonn-node_0.3.0_amd64.deb", int64(len(body)), sum, func(n int64) { last = n })
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, body) || last != int64(len(body)) {
		t.Fatalf("resumed file %d bytes, progress %d", len(got), last)
	}
	// A verified file is not downloaded again.
	if again, err := Fetch(context.Background(), nil, dir, "zonn-node_0.3.0_amd64.deb", int64(len(body)), sum, nil); err != nil || again != path {
		t.Fatalf("second fetch: %v", err)
	}

	// A corrupted download is deleted and never returned.
	bad := append([]byte{}, body...)
	bad[100] ^= 1
	if _, err := Fetch(context.Background(), serve(bad, 0), dir, "other.deb", int64(len(body)), sum, nil); !errors.Is(err, ErrChecksum) {
		t.Fatalf("corrupted download: %v", err)
	}
	for _, name := range []string{"other.deb", "other.deb.part"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s left behind", name)
		}
	}
	if _, err := Fetch(context.Background(), serve(body, 0), dir, "../escape.deb", 1, sum, nil); err == nil {
		t.Fatal("accepted a path as a file name")
	}
}

func TestOpenCommand(t *testing.T) {
	cases := map[string]string{"windows": `C:\u\setup.exe`, "darwin": "open", "linux": "xdg-open"}
	for goos, want := range cases {
		if name, _ := openCommand(goos, `C:\u\setup.exe`); name != want {
			t.Fatalf("%s: %s", goos, name)
		}
	}
	if p := Current(); p.Variant == "" || p.Format == "" {
		t.Fatalf("platform %+v", p)
	}
}
