package upload

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Bucket struct {
	client     *minio.Client
	bucketName string
}

func OpenBucket(envvar string) (*Bucket, error) {
	config := strings.Split(os.Getenv(envvar), ":")
	if len(config) != 4 {
		return nil, fmt.Errorf("could not find/parse %s", envvar)
	}

	client, err := minio.New(config[0], &minio.Options{
		Creds:  credentials.NewStaticV4(config[1], config[2], ""),
		Secure: true,
	})
	if err != nil {
		return nil, err
	}

	bucket := Bucket{client, config[3]}
	if err := bucket.CheckBucket(); err != nil {
		panic(err)
	}
	return &bucket, nil
}

func (b *Bucket) CheckBucket() error {
	ok, err := b.client.BucketExists(context.Background(), b.bucketName)
	if err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such bucket: %s", b.bucketName)
	}
	return nil
}

func (b *Bucket) PutFile(to string, from string, contentType, contentEncoding string) error {
	opts := minio.PutObjectOptions{
		ContentType:     contentType,
		ContentEncoding: contentEncoding,
	}
	_, err := b.client.FPutObject(context.Background(), b.bucketName, to, from, opts)
	return err
}

func (b *Bucket) PutBuffer(to string, buf *bytes.Buffer, contentType, contentEncoding string) error {
	opts := minio.PutObjectOptions{
		ContentType:     contentType,
		ContentEncoding: contentEncoding,
		// Workaround for https://github.com/minio/minio-go/issues/1791
		SendContentMd5: true,
	}
	_, err := b.client.PutObject(context.Background(), b.bucketName, to, buf,
		int64(buf.Len()), opts)
	return err
}

func (b *Bucket) PutBytes(to string, data []byte, contentType, contentEncoding string) error {
	buf := bytes.NewBuffer(data)
	return b.PutBuffer(to, buf, contentType, contentEncoding)
}

func (b *Bucket) ListObjects() <-chan minio.ObjectInfo {
	return b.client.ListObjects(context.Background(), b.bucketName,
		minio.ListObjectsOptions{Recursive: true})
}

func (b *Bucket) DeleteFiles(targets []*minio.ObjectInfo, start, total int) int {
	objects := make(chan minio.ObjectInfo)
	go func() {
		defer close(objects)
		for _, obj := range targets {
			objects <- *obj
		}
	}()

	var errored bool
	for err := range b.client.RemoveObjects(context.Background(), b.bucketName,
		objects, minio.RemoveObjectsOptions{}) {

		fmt.Printf("error: %v", err)
		errored = true
	}
	if errored {
		panic(fmt.Errorf("errors deleting files (see above)"))
	}
	return start
}
