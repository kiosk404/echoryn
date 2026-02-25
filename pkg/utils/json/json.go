package json

import (
	"encoding/json"

	"github.com/bytedance/sonic"
)

type RawMessage = json.RawMessage

func Marshal(v interface{}) ([]byte, error) {
	return sonic.Marshal(v)
}

func Unmarshal(data []byte, v interface{}) error {
	return sonic.Unmarshal(data, v)
}
