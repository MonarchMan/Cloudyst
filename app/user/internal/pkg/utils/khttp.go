package utils

import (
	"fmt"

	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func HeaderFromContext(ctx khttp.Context) (reqHeader, respHeader transport.Header, err error) {
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
