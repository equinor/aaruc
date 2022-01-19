package controllers

import (
	"bytes"
	"encoding/gob"
)

type Entry struct {
	AppId string
	Uri   string
}

func (e *Entry) Serialize() (*bytes.Buffer, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(&e); err != nil {
		return &buf, err
	}
	return &buf, nil
}

func DecodeEntry(buf *bytes.Buffer) (*Entry, error) {
	e := new(Entry)
	decoder := gob.NewDecoder(buf)
	if err := decoder.Decode(e); err != nil {
		return e, err
	}
	return e, nil
}
