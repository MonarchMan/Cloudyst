package utils

import (
	"io"

	"google.golang.org/grpc"
)

// StreamReader 将 gRPC stream 包装成 io.ReadCloser
type StreamReader struct {
	recvFn   func() ([]byte, error)
	buf      []byte
	received int64
	done     bool
}

func NewStreamReader[Req, Res any](
	stream grpc.ClientStreamingServer[Req, Res],
	extractChunk func(*Req) ([]byte, bool),
) *StreamReader {
	return &StreamReader{
		recvFn: func() ([]byte, error) {
			req, err := stream.Recv()
			if err != nil {
				return nil, err
			}
			data, ok := extractChunk(req)
			if !ok {
				return nil, nil // 非 chunk 包跳过
			}
			return data, nil
		},
	}
}

func (r *StreamReader) Read(p []byte) (n int, err error) {
	if len(r.buf) > 0 {
		n = copy(p, r.buf)
		r.buf = r.buf[n:]
		r.received += int64(n)
		return n, nil
	}
	if r.done {
		return 0, io.EOF
	}

	data, err := r.recvFn()
	if err == io.EOF {
		r.done = true
		return 0, io.EOF
	}
	if err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return r.Read(p) // 跳过 file_info 等非 chunk 包，递归读下一个
	}

	n = copy(p, data)
	if n < len(data) {
		r.buf = append(r.buf[:0], data[n:]...)
	}
	r.received += int64(n)
	return n, nil
}

func (r *StreamReader) Close() error {
	r.done = true
	r.buf = nil
	return nil
}

func (r *StreamReader) ReceivedSize() int64 {
	return r.received
}
