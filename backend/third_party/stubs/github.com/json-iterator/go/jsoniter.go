package jsoniter

import (
	"encoding/json"
	"io"
)

type API struct{}

var ConfigCompatibleWithStandardLibrary API

func (API) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (API) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (API) MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

func (API) NewDecoder(r io.Reader) *json.Decoder {
	return json.NewDecoder(r)
}

func (API) NewEncoder(w io.Writer) *json.Encoder {
	return json.NewEncoder(w)
}
