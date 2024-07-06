package upload

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/hashicorp/go-retryablehttp"
)

type Bucket struct {
	url       *url.URL // https://HOST/ZONE
	accessKey string
	client    *retryablehttp.Client
}

func OpenBucket(envvar string) (*Bucket, error) {
	url, err := url.Parse(os.Getenv(envvar))
	if err != nil {
		return nil, err
	}

	if url.User == nil {
		return nil, fmt.Errorf("expected password in url")
	}
	accessKey, ok := url.User.Password()
	if !ok {
		return nil, fmt.Errorf("expected password in url")
	}
	url.User = nil

	client := retryablehttp.NewClient()
	client.Logger = nil
	bucket := Bucket{url, accessKey, client}
	if err := bucket.CheckBucket(); err != nil {
		return nil, err
	}
	return &bucket, nil
}

func (b *Bucket) CheckBucket() error {
	req, err := b.newRequest("GET", nil, "/")
	if err != nil {
		return err
	}
	return b.doRequest(req)
}

func (b *Bucket) Put(to string, reader io.Reader, contentType string) error {
	req, err := b.newRequest("PUT", reader, to)
	if err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	return b.doRequest(req)
}

func (b *Bucket) Get(path string) (*bytes.Buffer, error) {
	req, err := b.newRequest("GET", nil, path)
	if err != nil {
		return nil, err
	}

	res, err := b.client.Do(req)
	if err != nil {
		return nil, err
	} else if res.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected response code %d", res.StatusCode)
	}

	defer res.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(res.Body)
	return &buf, nil
}

func (b *Bucket) newRequest(method string, body io.Reader, parts ...string) (*retryablehttp.Request, error) {
	path, err := url.JoinPath(b.url.String(), parts...)
	if err != nil {
		return nil, err
	}
	req, err := retryablehttp.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("AccessKey", b.accessKey)
	return req, nil
}

func (b *Bucket) doRequest(req *retryablehttp.Request) error {
	res, err := b.client.Do(req)
	if err != nil {
		return err
	} else if res.StatusCode == 200 {
		return nil
	} else if res.StatusCode == 201 {
		return nil
	}
	return fmt.Errorf("unexpected response code %d", res.StatusCode)
}
