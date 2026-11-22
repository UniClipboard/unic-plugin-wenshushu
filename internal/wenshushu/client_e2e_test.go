package wenshushu_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/UniClipboard/unic-plugin-wenshushu/internal/wenshushu"
)

func fixtures() []wenshushu.File {
	var img bytes.Buffer
	png.Encode(&img, image.NewGray(image.Rect(0, 0, 2, 2)))
	data := [][]byte{[]byte("UniClipboard synthetic 文叔叔 E2E\n"), img.Bytes(), bytes.Repeat([]byte{0, 1, 255, 42}, 600000)}
	names := []string{"clipboard.txt", "image.png", "中文 文件.bin"}
	var out []wenshushu.File
	for i, b := range data {
		out = append(out, wenshushu.File{Name: names[i], Size: int64(len(b)), Open: func(ctx context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }})
	}
	return out
}

func TestProviderE2E(t *testing.T) {
	var uploaded bytes.Buffer
	fail := ""
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/put" {
			if fail == "put" {
				w.WriteHeader(503)
				return
			}
			io.Copy(&uploaded, r.Body)
			return
		}
		var in map[string]any
		if r.Method == "POST" {
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Error(err)
			}
		}
		data := map[string]any{}
		switch r.URL.Path {
		case "/ap/login/anonymous":
			data["token"] = "synthetic-token"
		case "/ag/time":
			data["time"] = "1732000000123"
		case "/ap/task/addsend":
			if r.Header.Get("A-code") == "" {
				t.Error("missing signature")
			}
			data = map[string]any{"bid": "box", "ufileid": "parent", "tid": "task"}
		case "/ap/uploadv2/getupid":
			data["upId"] = "upload"
		case "/ap/uploadv2/psurl":
			data["url"] = base + "/put"
		case "/ap/uploadv2/fast":
			data["status"] = 0
		case "/ap/ufile/getprocess":
			data["rst"] = "success"
		case "/ap/uploadv2/complete":
		case "/ap/task/copysend":
			data["public_url"] = "https://www.wenshushu.cn/f/synthetic"
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if fail == "json" {
			fmt.Fprint(w, "invalid")
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}))
	defer server.Close()
	base = server.URL
	c := wenshushu.New(server.Client(), base)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	files := fixtures()
	url, err := c.Share(ctx, files)
	if err != nil || url != "https://www.wenshushu.cn/f/synthetic" {
		t.Fatalf("share: %v", err)
	}
	var expected []byte
	for _, f := range files {
		r, _ := f.Open(ctx)
		b, _ := io.ReadAll(r)
		r.Close()
		expected = append(expected, b...)
	}
	if !bytes.Equal(expected, uploaded.Bytes()) {
		t.Fatal("uploaded bytes differ")
	}
	for _, mode := range []string{"put", "json"} {
		fail = mode
		if _, err = c.Share(ctx, files); err == nil {
			t.Fatalf("%s should fail", mode)
		}
	}
	fail = ""
	for _, name := range []string{"../escape", "", `C:\\secret`, "a/b"} {
		bad := fixtures()
		bad[0].Name = name
		if _, err = c.Share(ctx, bad); err == nil {
			t.Fatalf("unsafe name accepted")
		}
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err = c.Share(canceled, files); err == nil {
		t.Fatal("cancellation ignored")
	}
	writeReceipt(t, "fixture", map[string]any{"status": "passed", "sha256": fmt.Sprintf("%x", sha256.Sum256(expected)), "bytes": len(expected)})
}

func writeReceipt(t *testing.T, name string, data any) {
	t.Helper()
	dir := os.Getenv("WENSHUSHU_RECEIPTS")
	if dir == "" {
		dir = t.TempDir()
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, name+".json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLiveProviderE2E(t *testing.T) {
	if os.Getenv("WENSHUSHU_LIVE") != "1" {
		t.Skip("set WENSHUSHU_LIVE=1 for synthetic live upload/download")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 30 * time.Second}
	c := wenshushu.New(client, "https://www.wenshushu.cn")
	files := fixtures()
	share, err := c.Share(ctx, files)
	if err != nil {
		writeReceipt(t, "live-first-failure", map[string]any{"stage": "upload", "error": err.Error()})
		t.Fatal(err)
	}
	// An independent anonymous recipient resolves the returned public share.
	recipientToken := ""
	call := func(path string, in any) map[string]any {
		t.Helper()
		b, _ := json.Marshal(in)
		req, _ := http.NewRequestWithContext(ctx, "POST", "https://www.wenshushu.cn"+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0")
		req.Header.Set("Accept-Language", "en-US, en;q=0.9")
		if recipientToken != "" {
			req.Header.Set("X-TOKEN", recipientToken)
		}
		res, e := client.Do(req)
		if e != nil {
			t.Fatal("recipient request failed")
		}
		defer res.Body.Close()
		var env struct {
			Code int            `json:"code"`
			Data map[string]any `json:"data"`
		}
		if json.NewDecoder(res.Body).Decode(&env) != nil || env.Code != 0 {
			t.Fatalf("recipient stage %s rejected: %d", path, env.Code)
		}
		return env.Data
	}
	recipientToken = call("/ap/login/anonymous", map[string]any{"dev_info": "{}"})["token"].(string)
	task := share[strings.LastIndex(share, "/")+1:]
	if len(task) != 11 {
		task = call("/ap/task/token", map[string]any{"token": task})["tid"].(string)
	}
	info := call("/ap/task/mgrtask", map[string]any{"tid": task, "password": ""})
	list := call("/ap/ufile/list", map[string]any{"start": 0, "sort": map[string]string{"name": "asc"}, "bid": info["boxid"], "pid": info["ufileid"], "type": 1, "options": map[string]string{"uploader": "true"}, "size": 50})
	entries := list["fileList"].([]any)
	if len(entries) != len(files) {
		t.Fatalf("recipient file count mismatch: got %d expected %d", len(entries), len(files))
	}
	hashes := map[string]string{}
	for _, entry := range entries {
		m := entry.(map[string]any)
		download := call("/ap/dl/sign", map[string]any{"consumeCode": 0, "type": 1, "ufileid": m["fid"]})["url"].(string)
		req, _ := http.NewRequestWithContext(ctx, "GET", download, nil)
		res, e := client.Do(req)
		if e != nil {
			t.Fatal("download failed")
		}
		b, e := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		if e != nil || res.StatusCode != 200 {
			t.Fatal("download rejected")
		}
		name := m["fname"].(string)
		matched := false
		for _, f := range files {
			if f.Name == name {
				r, _ := f.Open(ctx)
				expected, _ := io.ReadAll(r)
				r.Close()
				if !bytes.Equal(expected, b) {
					t.Fatal("recipient bytes differ")
				}
				matched = true
			}
		}
		if !matched {
			t.Fatal("unexpected recipient filename")
		}
		hashes[name] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	writeReceipt(t, "live", map[string]any{"status": "passed", "file_sha256": hashes, "expiry": info["expire"], "share_url": "[redacted]"})
}
