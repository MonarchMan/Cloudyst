package response

import (
	pbfile "api/api/file/files/v1"
	"api/external/trans"
	"common/serializer"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/encoding"
)

type testEmbed struct {
	Level1a int `json:"a"`
	Level1b int `json:"b"`
	Level1c int `json:"c"`
}

type testMessage struct {
	Field1 string     `json:"a"`
	Field2 string     `json:"b"`
	Field3 string     `json:"c"`
	Embed  *testEmbed `json:"embed,omitempty"`
}

type mock struct {
	value int
}

const (
	Unknown = iota
	Gopher
	Zebra
)

func (a *mock) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch strings.ToLower(s) {
	default:
		a.value = Unknown
	case "gopher":
		a.value = Gopher
	case "zebra":
		a.value = Zebra
	}

	return nil
}

func (a *mock) MarshalJSON() ([]byte, error) {
	var s string
	switch a.value {
	default:
		s = "unknown"
	case Gopher:
		s = "gopher"
	case Zebra:
		s = "zebra"
	}

	return json.Marshal(s)
}

func TestJSON_Marshal(t *testing.T) {
	codec := encoding.GetCodec("json")
	tests := []struct {
		input  any
		expect string
	}{
		{
			input: &trans.Response{
				Code: 200,
				Data: &pbfile.EntityUrl{Url: ""},
			},
			expect: `{"code":200,"data":{"url":""}}`,
		},
		{
			input: &serializer.Response{
				Code: 200,
				Data: &pbfile.EntityUrl{Url: "123"},
			},
			expect: `{"code":200,"data":{"url":"123"}}`,
		},
		{
			input:  &testMessage{},
			expect: `{"a":"","b":"","c":""}`,
		},
		{
			input:  &testMessage{Field1: "a", Field2: "b", Field3: "c"},
			expect: `{"a":"a","b":"b","c":"c"}`,
		},
		{
			input:  &mock{value: Gopher},
			expect: `"gopher"`,
		},
	}
	for _, v := range tests {
		data, err := codec.Marshal(v.input)
		if err != nil {
			t.Errorf("marshal(%#v): %s", v.input, err)
		}
		if got, want := string(data), v.expect; strings.ReplaceAll(got, " ", "") != want {
			if strings.Contains(want, "\n") {
				t.Errorf("marshal(%#v):\nHAVE:\n%s\nWANT:\n%s", v.input, got, want)
			} else {
				t.Errorf("marshal(%#v):\nhave %#q\nwant %#q", v.input, got, want)
			}
		}
	}
}
