package fzf

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/btidor/src.codes/internal"
)

func TestServerA(t *testing.T) {
	server := Server{
		Meta:        internal.URLMustParse("https://meta.src.codes"),
		Commit:      "dev",
		ResultLimit: 100,
	}
	server.EnsureIndex("hirsute")

	r := httptest.NewRequest("GET", "http://localhost/hirsute?q=abseilabsl.c", nil)
	w := httptest.NewRecorder()
	server.Handle(w, r, true)
	ct := strings.SplitN(w.Body.String(), " ", 2)[0]
	if ct != "151" {
		t.Errorf("Incorrect count: %s", ct)
	}
}

func TestServerB(t *testing.T) {
	server := Server{
		Meta:        internal.URLMustParse("https://meta.src.codes"),
		Commit:      "dev",
		ResultLimit: 100,
	}
	server.EnsureIndex("hirsute")

	r := httptest.NewRequest("GET", "http://localhost/hirsute?q=baseinternalthreadidentity", nil)
	w := httptest.NewRecorder()
	server.Handle(w, r, true)
	ct := strings.SplitN(w.Body.String(), " ", 2)[0]
	if ct != "456" {
		t.Errorf("Incorrect count: %s", ct)
	}
}
