package internal

import (
	"io"
	"net/http/httptest"
	"testing"
)

func TestURLMustParseSuccess(t *testing.T) {
	u1 := URLMustParse("https://src.codes/")
	if u1.Scheme != "https" || u1.Host != "src.codes" || u1.Path != "/" {
		t.Errorf("Mis-parsed URL: %#v", u1)
	}

	u2 := URLMustParse("https://fzf.src.codes/hello?q=123")
	if u2.Host != "fzf.src.codes" || u2.Path != "/hello" || u2.Query()["q"][0] != "123" {
		t.Errorf("Mis-parsed URL: %#v", u2)
	}
}

func TestURLMustParseFailure(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Errorf("URLMustParse did not panic")
		}
	}()

	URLMustParse("very&%invalid")
}

func TestURLWithPath(t *testing.T) {
	base := URLMustParse("https://src.codes/hello")

	u := URLWithPath(base, "foo", "bar.txt")
	if u.Host != "src.codes" || u.Path != "/hello/foo/bar.txt" {
		t.Errorf("Mis-joined URL: %#v", u)
	}
}

func TestHTTPError(t *testing.T) {
	req1 := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w1 := httptest.NewRecorder()
	HTTPError(w1, req1, 404)

	res1 := w1.Result()
	body1, _ := io.ReadAll(res1.Body)
	if res1.StatusCode != 404 || string(body1) != "404 Not Found\n" {
		t.Errorf("Incorrect response: %#v %#v", res1, body1)
	}

	req2 := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w2 := httptest.NewRecorder()
	HTTPError(w2, req2, 500)

	res2 := w2.Result()
	body2, _ := io.ReadAll(res2.Body)
	if res2.StatusCode != 500 || string(body2) != "500 Internal Server Error\n" {
		t.Errorf("Incorrect response: %#v %#v", res2, body2)
	}
}
