package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"airbridge/internal/store"
)

type testClient struct {
	t       *testing.T
	handler http.Handler
	cookie  *http.Cookie
}

func client(t *testing.T) *testClient {
	t.Helper()
	s, err := New(Config{Address: "127.0.0.1:8080", Directory: t.TempDir(), PIN: "123456", Name: "Test room", MaxFile: 16 << 20, Quota: 32 << 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return &testClient{t: t, handler: s.Handler()}
}
func (c *testClient) request(method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, req)
	return w
}
func (c *testClient) join() {
	c.t.Helper()
	w := c.request("POST", "/api/join", []byte(`{"pin":"123456","name":"Test device"}`), nil)
	if w.Code != 200 {
		c.t.Fatal(w.Code, w.Body.String())
	}
	c.cookie = w.Result().Cookies()[0]
	if !c.cookie.HttpOnly || c.cookie.SameSite != http.SameSiteStrictMode {
		c.t.Fatal("unsafe cookie")
	}
}
func TestAccessControls(t *testing.T) {
	c := client(t)
	for _, path := range []string{"/api/room", "/api/files", "/api/files/unknown/download"} {
		if w := c.request("GET", path, nil, nil); w.Code != 401 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	if w := c.request("POST", "/api/join", []byte(`{"pin":"123456"}`), map[string]string{"Origin": "http://evil.test"}); w.Code != 403 {
		t.Fatal("cross origin accepted")
	}
	req := httptest.NewRequest("GET", "http://evil.test/api/room", nil)
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("DNS rebinding host accepted")
	}
	c.join()
	if w := c.request("GET", "/api/room", nil, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := c.request("POST", "/api/leave", nil, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := c.request("GET", "/api/files", nil, nil); w.Code != 401 {
		t.Fatal("revoked session accepted")
	}
}
func TestTransferContract(t *testing.T) {
	c := client(t)
	c.join()
	payload := bytes.Repeat([]byte("Airbridge\x00"), 1000000)
	meta, _ := json.Marshal(map[string]any{"name": "日本語 report.bin", "size": len(payload)})
	w := c.request("POST", "/api/files", meta, nil)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var f store.File
	if err := json.Unmarshal(w.Body.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	path := "/api/files/" + f.ID
	if w := c.request("GET", path+"/download", nil, nil); w.Code != 409 {
		t.Fatal("incomplete download allowed")
	}
	if w := c.request("POST", path+"/complete", nil, nil); w.Code != 409 {
		t.Fatal("incomplete publication allowed")
	}
	for offset := 0; offset < len(payload); {
		end := min(offset+int(store.ChunkSize), len(payload))
		w = c.request("PATCH", path, payload[offset:end], map[string]string{"Upload-Offset": fmt.Sprint(offset)})
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		offset = end
	}
	if w := c.request("PATCH", path, []byte("a"), map[string]string{"Upload-Offset": "0"}); w.Code != 409 {
		t.Fatal("stale write accepted")
	}
	if w := c.request("POST", path+"/complete", nil, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = c.request("GET", path+"/download", nil, nil)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), payload) {
		t.Fatal("download differs")
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("missing attachment header")
	}
	w = c.request("GET", path+"/download", nil, map[string]string{"Range": "bytes=7-100"})
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), payload[7:101]) {
		t.Fatal("range mismatch", w.Code)
	}
	w = c.request("HEAD", path+"/download", nil, nil)
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != fmt.Sprint(len(payload)) {
		t.Fatal("HEAD contract", w.Code)
	}
	if w := c.request("DELETE", path, nil, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := c.request("GET", path+"/download", nil, nil); w.Code != 404 {
		t.Fatal("deleted file available")
	}
}
func TestJoinLimitAndValidation(t *testing.T) {
	c := client(t)
	for i := 0; i < 10; i++ {
		if w := c.request("POST", "/api/join", []byte(`{"pin":"bad"}`), nil); w.Code != 401 {
			t.Fatal(i, w.Code)
		}
	}
	if w := c.request("POST", "/api/join", []byte(`{"pin":"123456"}`), nil); w.Code != 429 {
		t.Fatal("join limit missing")
	}
	c = client(t)
	c.join()
	for _, body := range []string{`{"name":"../escape","size":1}`, `{"name":"big","size":999999999}`, `{"name":"negative","size":-1}`, `{"name":"ok","size":1} {}`} {
		if w := c.request("POST", "/api/files", []byte(body), nil); w.Code != 400 {
			t.Fatal("invalid creation", body, w.Code)
		}
	}
	if w := c.request("POST", "/api/fingerprint", bytes.Repeat([]byte{1}, (128<<10)+1), nil); w.Code != 400 {
		t.Fatal("sample limit missing")
	}
}
