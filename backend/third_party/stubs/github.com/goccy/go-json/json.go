package json

import (
	stdjson "encoding/json"
	"io"
)

func Marshal(v any) ([]byte, error) {
	return stdjson.Marshal(v)
}

func Unmarshal(data []byte, v any) error {
	return stdjson.Unmarshal(data, v)
}

func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return stdjson.MarshalIndent(v, prefix, indent)
}

func NewDecoder(r io.Reader) *stdjson.Decoder {
	return stdjson.NewDecoder(r)
}

func NewEncoder(w io.Writer) *stdjson.Encoder {
	return stdjson.NewEncoder(w)
}
