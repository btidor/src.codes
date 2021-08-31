package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/ProtonMail/gopenpgp/v2/helper"
)

// ConstructURL builds a URL from a base string and a sequence of path
// components. If the base string is not a valid URL, it panics.
func ConstructURL(mirror string, parts ...string) *url.URL {
	url, err := url.Parse(mirror)
	if err != nil {
		panic(err)
	}
	pathComponents := make([]string, len(parts)+1)
	pathComponents[0] = url.Path
	for i, p := range parts {
		pathComponents[i+1] = p
	}
	url.Path = path.Join(pathComponents...)
	return url
}

// DownloadAndVerifyArmored downloads an armored PGP-signed file from the given
// URL. If the signature is invalid or a network error occurs, it returns an
// error. Otherwise, it returns the data under the signature.
func DownloadAndVerifyArmored(url *url.URL, keyfile string) (string, error) {
	key, err := os.ReadFile(keyfile)
	if err != nil {
		return "", err
	}

	response, err := http.Get(url.String())
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("could not download %s: HTTP %s", url.String(), response.Status)
	}

	buffer := new(bytes.Buffer)
	buffer.ReadFrom(response.Body)

	return helper.VerifyCleartextMessageArmored(
		string(key), buffer.String(), crypto.GetUnixTime(),
	)
}

// DownloadAndVerifySHA256 downloads a file from the given URL. If the hash is
// invalid or a network error occurs, it returns an error. Otherwise, it returns
// the data under the signature.
func DownloadAndVerifySHA256(url *url.URL, hash string) (*bytes.Buffer, error) {
	response, err := http.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("could not download %s: HTTP %s", url.String(), response.Status)
	}

	buffer := new(bytes.Buffer)
	buffer.ReadFrom(response.Body)

	h := sha256.New()
	h.Write(buffer.Bytes())
	actual := fmt.Sprintf("%x", h.Sum(nil))

	if actual != hash {
		return nil, fmt.Errorf("hash mismatch when downloading %s: got %s, expected %s", url.String(), actual, hash)
	}
	return buffer, nil
}

func DownloadToFileAndMD5(url *url.URL, filepath, hash string) error {
	response, err := http.Get(url.String())
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("could not download %s: HTTP %s", url.String(), response.Status)
	}

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	h := md5.New()

	buf := make([]byte, 32*1024)
	for {
		n, err := response.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[0:n]); err != nil {
				return err
			}
			if _, err := h.Write(buf[0:n]); err != nil {
				return err
			}
		}
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}
	actual := fmt.Sprintf("%x", h.Sum(nil))
	if actual != hash {
		return fmt.Errorf("hash mismatch when downloading %s: got %s, expected %s", url.String(), actual, hash)
	}
	return nil
}

func ExtractArchive(source, destination string, stripComponents int) error {
	stripArg := fmt.Sprintf("--strip-components=%d", stripComponents)
	out, err := exec.Command("tar", "-xf", source, "--directory", destination, stripArg).CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %#v\noutput: %s", err, out)
	}
	return nil
}
