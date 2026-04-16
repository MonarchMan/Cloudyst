package response

import (
	"api/external/trans"
	xerrors "common/errors"
	"common/util"
	"net/http"

	"github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func ErrorEncoder(w http.ResponseWriter, r *http.Request, err error) {
	se := errors.FromError(err)

	bizCode := xerrors.GetCode(se.Reason)
	resp := &trans.Response{
		Code:          bizCode,
		Msg:           se.Reason,
		Error:         se.Message,
		Metadata:      se.Metadata,
		CorrelationID: util.TraceID(r.Context()),
	}
	codec, _ := khttp.CodecForRequest(r, "Accept")
	body, err := codec.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/"+codec.Name())
	w.WriteHeader(int(se.Code))
	_, _ = w.Write(body)
}

func ResponseEncoder(w http.ResponseWriter, r *http.Request, v interface{}) error {
	if v == nil {
		v = struct{}{}
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

	w.Header().Set("Content-Type", "application/"+codec.Name())
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(data)
	return err
}
