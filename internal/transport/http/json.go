package httptransport

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// readJSON accepts exactly one JSON value within the endpoint's byte limit.
// Unknown fields remain compatible with older clients unless the endpoint
// explicitly uses readStrictJSON.
func readJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) error {
	return readRequestJSON(w, r, maxBytes, dst, false)
}

func readStrictJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) error {
	return readRequestJSON(w, r, maxBytes, dst, true)
}

func readRequestJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any, strict bool) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	// A successful first Decode does not consume a second document or trailing
	// garbage, and can otherwise accept a body larger than its byte limit.
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("request body must contain one JSON value")
	}
	return nil
}
