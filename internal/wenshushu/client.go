// Package wenshushu implements the anonymous temporary-share HTTP protocol.
package wenshushu

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/des"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mr-tron/base58"
)

const chunkSize = 2 << 20
const MaxBytes int64 = 5 << 30
const MaxFiles = 500

// File is a source opened only during Share. The caller owns its lifetime.
type File struct {
	Name string
	Size int64
	Open func(context.Context) (io.ReadCloser, error)
}

type Client struct {
	http *http.Client
	base string
}

func New(client *http.Client, base string) *Client {
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	// Never follow redirects carrying credentials or mutation bodies.
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{http: &copy, base: strings.TrimRight(base, "/")}
}

// Error intentionally excludes provider bodies, URLs and source content.
type Error struct {
	Stage string
	Code  int
}

func (e *Error) Error() string { return fmt.Sprintf("wenshushu: %s failed (code %d)", e.Stage, e.Code) }

func ValidName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 255 && utf8.ValidString(name) && !strings.ContainsAny(name, "/\\:") && !strings.ContainsFunc(name, unicode.IsControl)
}

// Share creates exactly one share. Provider mutations are never retried.
func (c *Client) Share(ctx context.Context, files []File) (string, error) {
	if len(files) == 0 || len(files) > MaxFiles {
		return "", errors.New("wenshushu: invalid file count")
	}
	total := int64(0)
	names := map[string]bool{}
	for _, f := range files {
		if names[f.Name] || !ValidName(f.Name) || f.Size < 0 || f.Size > MaxBytes-total || f.Open == nil {
			return "", errors.New("wenshushu: invalid source")
		}
		names[f.Name] = true
		total += f.Size
	}
	var login struct {
		Token string `json:"token"`
	}
	if err := c.call(ctx, "login/anonymous", "", map[string]any{"dev_info": "{}"}, nil, &login); err != nil {
		return "", err
	}
	if login.Token == "" {
		return "", &Error{"login", 0}
	}
	token := login.Token
	var clock struct {
		Time string `json:"time"`
	}
	if err := c.request(ctx, http.MethodGet, "/ag/time", "", nil, nil, &clock); err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{"sender": "", "remark": "", "isextension": false, "notSaveTo": false, "notDownload": false, "notPreview": false, "downPreCountLimit": 0, "trafficStatus": 0, "pwd": "", "expire": "1", "recvs": []string{"social", "public"}, "file_size": total, "file_count": len(files)})
	if err != nil {
		return "", err
	}
	signature, err := sign(clock.Time, token, body)
	if err != nil {
		return "", err
	}
	var task struct {
		Box    string `json:"bid"`
		Parent string `json:"ufileid"`
		ID     string `json:"tid"`
	}
	if err = c.request(ctx, http.MethodPost, "/ap/task/addsend", token, body, map[string]string{"A-code": signature, "Req-Time": clock.Time}, &task); err != nil {
		return "", err
	}
	if task.Box == "" || task.Parent == "" || task.ID == "" {
		return "", &Error{"task", 0}
	}
	var up struct {
		ID string `json:"upId"`
	}
	if err = c.call(ctx, "uploadv2/getupid", token, map[string]any{"preid": task.Parent, "boxid": task.Box, "linkid": task.ID, "utype": "sendcopy", "originUpid": "", "length": total, "count": len(files)}, nil, &up); err != nil {
		return "", err
	}
	if up.ID == "" {
		return "", &Error{"upload ID", 0}
	}
	for _, f := range files {
		if err = c.upload(ctx, token, task.Box, task.Parent, up.ID, f); err != nil {
			return "", err
		}
	}
	if err = c.wait(ctx, token, up.ID); err != nil {
		return "", err
	}
	var result struct {
		URL string `json:"public_url"`
	}
	if err = c.call(ctx, "task/copysend", token, map[string]string{"bid": task.Box, "tid": task.ID, "ufileid": task.Parent}, nil, &result); err != nil {
		return "", err
	}
	if !publicURL(result.URL) {
		return "", &Error{"share URL", 0}
	}
	return result.URL, nil
}

func (c *Client) upload(ctx context.Context, token, box, parent, uploadID string, f File) error {
	r, err := f.Open(ctx)
	if err != nil {
		return &Error{"source open", 0}
	}
	defer r.Close()
	multipart := f.Size > chunkSize
	first := make([]byte, int(min(int64(chunkSize), f.Size)))
	if _, err = io.ReadFull(r, first); err != nil {
		return &Error{"source read", 0}
	}
	if !multipart {
		var extra [1]byte
		if n, e := io.ReadFull(r, extra[:]); n != 0 || e != io.EOF {
			return &Error{"source size", 0}
		}
	}
	md := md5.Sum(first)
	sh := sha1.Sum(first)
	mdHex := hex.EncodeToString(md[:])
	h := map[string]string{"cm1": mdHex, "cs1": hex.EncodeToString(sh[:])}
	if !multipart {
		cm := sha1.Sum([]byte(mdHex))
		h["cm"] = hex.EncodeToString(cm[:])
	}
	var fast struct {
		Status int             `json:"status"`
		File   json.RawMessage `json:"ufile"`
	}
	if err = c.call(ctx, "uploadv2/fast", token, map[string]any{"hash": h, "uf": map[string]string{"name": f.Name, "boxid": box, "preid": parent}, "upId": uploadID}, nil, &fast); err != nil {
		return err
	}
	var fastFile map[string]json.RawMessage
	if fast.Status != 0 && json.Unmarshal(fast.File, &fastFile) == nil && len(fastFile) > 0 {
		// Dedup registers this file in the batch; wait only after every file is admitted.
		return nil
	}
	remaining := f.Size
	for part := 1; remaining > 0 || part == 1; part++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		n := min(int64(chunkSize), remaining)
		b := first[:int(n)]
		if part > 1 {
			if _, err = io.ReadFull(r, b); err != nil {
				return &Error{"source read", 0}
			}
		}
		in := map[string]any{"ispart": multipart, "fname": f.Name, "fsize": n, "upId": uploadID}
		if multipart {
			in["partnu"] = part
		}
		var target struct {
			URL string `json:"url"`
		}
		if err = c.call(ctx, "uploadv2/psurl", token, in, nil, &target); err != nil {
			return err
		}
		u, e := url.Parse(target.URL)
		if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && !(strings.HasPrefix(c.base, "http://127.0.0.1:") && strings.HasPrefix(target.URL, c.base+"/"))) {
			return &Error{"upload URL", 0}
		}
		req, e := http.NewRequestWithContext(ctx, http.MethodPut, target.URL, bytes.NewReader(b))
		if e != nil {
			return &Error{"upload request", 0}
		}
		res, e := c.http.Do(req)
		if e != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return &Error{"upload", 0}
		}
		io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return &Error{"upload", res.StatusCode}
		}
		remaining -= n
	}
	var extra [1]byte
	if n, e := io.ReadFull(r, extra[:]); n != 0 || e != io.EOF {
		return &Error{"source size", 0}
	}
	return c.call(ctx, "uploadv2/complete", token, map[string]any{"ispart": multipart, "fname": f.Name, "upId": uploadID, "location": map[string]string{"boxid": box, "preid": parent}}, nil, nil)
}

func (c *Client) call(ctx context.Context, path, token string, in any, headers map[string]string, out any) error {
	b, e := json.Marshal(in)
	if e != nil {
		return &Error{path, 0}
	}
	return c.request(ctx, http.MethodPost, "/ap/"+path, token, b, headers, out)
}
func (c *Client) request(ctx context.Context, method, path, token string, body []byte, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return &Error{path, 0}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:82.0) Gecko/20100101 Firefox/82.0")
	req.Header.Set("Accept-Language", "en-US, en;q=0.9")
	req.Header.Set("Prod", "com.wenshushu.web.pc")
	req.Header.Set("Referer", c.base+"/")
	req.Header.Set("Origin", c.base)
	if token != "" {
		req.Header.Set("X-TOKEN", token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &Error{path, 0}
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return &Error{path, res.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return &Error{path, 0}
	}
	var env struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(b, &env) != nil || env.Code == nil {
		return &Error{path, 0}
	}
	if *env.Code != 0 {
		return &Error{path, *env.Code}
	}
	if out != nil && json.Unmarshal(env.Data, out) != nil {
		return &Error{path, 0}
	}
	return nil
}

func sign(stamp, token string, body []byte) (string, error) {
	if len(stamp) < 10 || strings.ContainsFunc(stamp, func(r rune) bool { return r < '0' || r > '9' }) {
		return "", &Error{"server time", 0}
	}
	key := make([]byte, 8)
	for i := 0; i < 5; i++ {
		key[i] = stamp[int(stamp[len(stamp)-1-i]-'0')]
	}
	copy(key[5:], "000")
	sum := md5.Sum(append(append([]byte{}, body...), token...))
	hexSum := make([]byte, 32)
	hex.Encode(hexSum, sum[:])
	raw := []byte(base58.Encode(hexSum))
	pad := 8 - len(raw)%8
	raw = append(raw, bytes.Repeat([]byte{byte(pad)}, pad)...)
	block, err := des.NewCipher(key)
	if err != nil {
		return "", &Error{"signature", 0}
	}
	cipher.NewCBCEncrypter(block, key).CryptBlocks(raw, raw)
	return base64.StdEncoding.EncodeToString(raw), nil
}
func publicURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/f/") || len(u.Path) <= 3 {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, domain := range []string{"wenshushu.cn", "wss1.cn", "wss.ink"} {
		if h == domain || strings.HasSuffix(h, "."+domain) {
			return true
		}
	}
	return false
}

func (c *Client) wait(ctx context.Context, token, id string) error {
	for {
		var state struct {
			Result string `json:"rst"`
		}
		if err := c.call(ctx, "ufile/getprocess", token, map[string]string{"processId": id}, nil, &state); err != nil {
			return err
		}
		if state.Result == "success" {
			return nil
		}
		if state.Result == "failed" || state.Result == "fail" || state.Result == "error" {
			return &Error{"upload processing", 0}
		}
		timer := time.NewTimer(300 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
