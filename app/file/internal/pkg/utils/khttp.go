package utils

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func HeaderFromContext(ctx context.Context) (reqHeader, respHeader transport.Header, err error) {
	tr, ok := transport.FromServerContext(ctx)
	if !ok {
		return nil, nil, fmt.Errorf("no transport available")
	}
	ht, ok := tr.(khttp.Transporter)
	if !ok {
		return nil, nil, fmt.Errorf("no http transport available")
	}
	reqHeader = ht.RequestHeader()
	respHeader = ht.ReplyHeader()
	return reqHeader, respHeader, nil
}

func RequestFromContext(ctx context.Context) (*http.Request, error) {
	tr, ok := transport.FromServerContext(ctx)
	if !ok {
		return nil, fmt.Errorf("no transport available")
	}
	ht, ok := tr.(khttp.Transporter)
	if !ok {
		return nil, fmt.Errorf("no http transport available")
	}
	return ht.Request(), nil
}

func WithValue(req *http.Request, key any, value any) {
	req = req.WithContext(context.WithValue(req.Context(), key, value))
}
