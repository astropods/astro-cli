package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v) //nolint:errchkjson
	}
}

func writeRawJSON(w http.ResponseWriter, status int, data json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_, _ = w.Write(data)
	}
}

func readJSON(r *http.Request, v interface{}) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	defer r.Body.Close() //nolint:errcheck
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeGRPCErr(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	httpStatus := http.StatusBadGateway
	if st.Code() == codes.NotFound {
		httpStatus = http.StatusNotFound
	}
	writeErr(w, httpStatus, st.Message())
}
