package process

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	share "github.com/UniClipboard/unic-plugin-wenshushu"
	"github.com/UniClipboard/unic-plugin-wenshushu/internal/wenshushu"
	"github.com/uniclipboard/unic/engine"
	"github.com/uniclipboard/unic/plugin"
)

func TestMain(m *testing.M) {
	if os.Getenv("WSS_PROCESS_HELPER") == "1" {
		p := share.New(wenshushu.New(nil, os.Getenv("WSS_PROCESS_ENDPOINT")))
		if plugin.Serve(p) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestPluginProcessE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var mu sync.Mutex
	var uploaded bytes.Buffer
	shares := 0
	fail := false
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if fail {
			w.WriteHeader(503)
			return
		}
		if r.URL.Path == "/put" {
			io.Copy(&uploaded, r.Body)
			return
		}
		d := map[string]any{}
		switch r.URL.Path {
		case "/ap/login/anonymous":
			d["token"] = "test-token"
		case "/ag/time":
			d["time"] = "1732000000123"
		case "/ap/task/addsend":
			shares++
			d = map[string]any{"bid": "box", "ufileid": "parent", "tid": "task"}
		case "/ap/uploadv2/getupid":
			d["upId"] = "upload"
		case "/ap/uploadv2/fast":
			d["status"] = 0
		case "/ap/uploadv2/psurl":
			d["url"] = base + "/put"
		case "/ap/ufile/getprocess":
			d["rst"] = "success"
		case "/ap/uploadv2/complete":
		case "/ap/task/copysend":
			d["public_url"] = "https://www.wenshushu.cn/f/synthetic"
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": d})
	}))
	defer server.Close()
	base = server.URL
	exe, _ := os.Executable()
	binary, _ := os.ReadFile(exe)
	sum := sha256.Sum256(binary)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.New(ctx, engine.Options{DataDir: dir, Plugins: []engine.PluginInstance{{Process: &engine.PluginProcess{Path: exe, SHA256: hex.EncodeToString(sum[:]), Spec: share.New(nil).Spec(), Args: []string{"-test.run=^$"}, Env: []string{"WSS_PROCESS_HELPER=1", "WSS_PROCESS_ENDPOINT=" + base}}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close(context.Background())
	if err = e.Start(ctx); err != nil {
		t.Fatal(err)
	}
	var imageData bytes.Buffer
	png.Encode(&imageData, image.NewGray(image.Rect(0, 0, 2, 2)))
	blob := bytes.Repeat([]byte{0, 1, 2, 255}, 600000)
	cases := []struct {
		name     string
		in       engine.ContentInput
		expected []byte
	}{
		{"text", engine.ContentInput{Representations: []engine.RepresentationInput{{MIME: engine.MimeText, Reader: bytes.NewBufferString("合成文本"), Size: 12}}}, []byte("合成文本")},
		{"image", engine.ContentInput{Representations: []engine.RepresentationInput{{MIME: "image/png", Reader: bytes.NewReader(imageData.Bytes()), Size: int64(imageData.Len())}}}, imageData.Bytes()},
		{"files", engine.ContentInput{Files: []engine.FileInput{{Name: "中文 文件.bin", Reader: bytes.NewReader(blob), Size: int64(len(blob))}, {Name: "empty.txt", Reader: bytes.NewReader(nil), Size: 0}}}, blob},
	}
	hashes := map[string]string{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mu.Lock()
			uploaded.Reset()
			before := shares
			mu.Unlock()
			h, err := e.ImportContent(ctx, tc.in)
			if err != nil {
				t.Fatal(err)
			}
			defer e.ReleaseContent(ctx, h.LeaseID)
			src, err := e.CommitContent(ctx, engine.CommitRequest{RequestID: engine.RequestID("source-" + tc.name), LeaseID: h.LeaseID})
			if err != nil {
				t.Fatal(err)
			}
			req := engine.RunActionRequest{RequestID: engine.RequestID("share-" + tc.name), Key: "unic.wenshushu/share", HistoryID: src.HistoryID}
			result, err := e.RunAction(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			again, err := e.RunAction(ctx, req)
			if err != nil || again.HistoryID != result.HistoryID {
				t.Fatal("duplicate request changed result")
			}
			item, err := e.GetItem(ctx, result.HistoryID)
			if err != nil || item.Origin.PluginID != "unic.wenshushu" {
				t.Fatal("invalid result origin", err)
			}
			lease, err := e.RetainHistoryContent(ctx, result.HistoryID)
			if err != nil {
				t.Fatal(err)
			}
			defer e.ReleaseContent(ctx, lease.LeaseID)
			r, err := e.ReadContent(ctx, lease.LeaseID, lease.Content.Parts[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(r)
			r.Close()
			if err != nil || string(b) != "https://www.wenshushu.cn/f/synthetic" {
				t.Fatal("wrong result text", err)
			}
			mu.Lock()
			matches := bytes.Equal(uploaded.Bytes(), tc.expected) && shares == before+1
			mu.Unlock()
			if !matches {
				t.Fatal("uploaded bytes or provider invocation count mismatch")
			}
			if _, err = e.GetItem(ctx, src.HistoryID); err != nil {
				t.Fatal("source changed")
			}
			hash := sha256.Sum256(tc.expected)
			hashes[tc.name] = hex.EncodeToString(hash[:])
			mu.Lock()
			fail = true
			mu.Unlock()
			failed, err := e.RunAction(ctx, engine.RunActionRequest{RequestID: engine.RequestID("fail-" + tc.name), Key: req.Key, HistoryID: src.HistoryID})
			if err == nil || failed.HistoryID != "" {
				t.Fatal("provider failure created success")
			}
			mu.Lock()
			fail = false
			mu.Unlock()
		})
	}
	if t.Failed() {
		return
	}
	receiptDir := os.Getenv("WENSHUSHU_RECEIPTS")
	if receiptDir == "" {
		receiptDir = t.TempDir()
	}
	os.MkdirAll(receiptDir, 0700)
	b, _ := json.MarshalIndent(map[string]any{"status": "passed", "payload_sha256": hashes, "cases": []string{"text", "image", "multipart-and-empty-files", "request-id-dedup", "provider-failure", "source-preserved"}, "replay": "go test -race ./e2e/process -count=1 -v"}, "", "  ")
	if err = os.WriteFile(filepath.Join(receiptDir, "process.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	fmt.Println("Wenshushu process E2E complete")
}
