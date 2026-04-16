package response

import (
	pbfile "api/api/file/files/v1"
	"api/external/trans"
	"common/util"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func ResponseEncoder(w http.ResponseWriter, r *http.Request, v interface{}) error {
	if strings.HasPrefix(r.URL.Path, "/dav") {
		return nil
	}

	if v == nil {
		v = struct{}{}
	}

	if resp, ok := v.(*pbfile.RedirectResponse); ok {
		http.Redirect(w, r, resp.Url, http.StatusFound)
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", resp.Expire))
	} else if resp, ok := v.(*pbfile.FileUrlResponse); ok && len(resp.Urls) == 1 {
		http.Redirect(w, r, resp.Urls[0].Url, http.StatusFound)
	}

	resp := &trans.Response{
		Code: 0,
		Data: v,
	}

	codec, _ := khttp.CodecForRequest(r, "Accept")
	data, err := codec.Marshal(resp)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(data)
	return err
}

func ErrorEncoder(w http.ResponseWriter, r *http.Request, err error) {
	// 【关键】WebDAV 的错误也不要包装成 JSON！
	// WebDAV 客户端期望看到标准的 HTTP 状态码和 XML 错误，而不是 JSON
	if strings.HasPrefix(r.URL.Path, "/dav/") {
		// 这里可以使用 khttp.DefaultErrorEncoder 或者简单的 http.Error
		// 为了简单，直接回退到 Kratos 默认行为，或者自己处理
		khttp.DefaultErrorEncoder(w, r, err)
		return
	}

	se := errors.FromError(err)
	resp := trans.Response{
		Code:          int(se.Code),
		Msg:           se.Message,
		Error:         se.Error(),
		CorrelationID: util.TraceID(r.Context()),
	}

	codec, _ := khttp.CodecForRequest(r, "Accept")
	body, err := codec.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/"+codec.Name())
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}
