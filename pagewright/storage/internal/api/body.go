package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Read the entire bounded representation before decoding, including trailing
// whitespace. A valid JSON prefix cannot hide an unlimited unread suffix.
func boundedJSON(r *http.Request, target any) error {
	data, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("expected one JSON value")
	}
	return nil
}
