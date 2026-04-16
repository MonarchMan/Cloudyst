package util

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// Exists reports whether the named files or directory exists.
func Exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// CreateNestedFile 给定path创建文件，如果目录不存在就递归创建
func CreateNestedFile(path string) (*os.File, error) {
	basePath := filepath.Dir(path)
	if !Exists(basePath) {
		err := os.MkdirAll(basePath, 0700)
		if err != nil {
			return nil, err
		}
	}

	return os.Create(path)
}

// CreateNestedFolder creates a folder with the given path, if the directory does not exist,
// it will be created recursively.
func CreateNestedFolder(path string) error {
	if !Exists(path) {
		err := os.MkdirAll(path, 0700)
		if err != nil {
			return err
		}
	}

	return nil
}

// IsEmpty 返回给定目录是否为空目录
func IsEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err // Either not empty or error, suits both cases
}

type CallbackReader struct {
	reader   io.Reader
	callback func(int64)
}

func NewCallbackReader(reader io.Reader, callback func(int64)) *CallbackReader {
	return &CallbackReader{
		reader:   reader,
		callback: callback,
	}
}

func (r *CallbackReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	r.callback(int64(n))
	return
}

func WriteSSE(ctx khttp.Context, event string, data interface{}) error {
	w := ctx.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("response writer is not flusher")
	}

	if event != "" {
		fmt.Fprintf(w, "event: %s\n", event)
	}

	// 简单处理 data 为 nil 的情况
	if data != nil {
		bytes, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", bytes)
	} else {
		fmt.Fprint(w, "data: \n\n")
	}
	flusher.Flush()
	return nil
}
