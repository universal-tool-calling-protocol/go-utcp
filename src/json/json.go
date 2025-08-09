package json

import (
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

var (
	Marshal    = json.Marshal
	Unmarshal  = json.Unmarshal
	NewDecoder = json.NewDecoder
	NewEncoder = json.NewEncoder
)

type RawMessage = jsoniter.RawMessage

type Decoder = jsoniter.Decoder

type Encoder = jsoniter.Encoder
