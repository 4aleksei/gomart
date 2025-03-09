package httplogs

import (
	"net/http"
)

type (
	responseData struct {
		status int
		size   int
	}

	loggingResponseWriter struct {
		http.ResponseWriter
		responseData *responseData
	}
)

func (r *responseData) GetSize() int {
	return r.size
}

func (r *responseData) GetStatus() int {
	return r.status
}

func (r *loggingResponseWriter) Write(b []byte) (int, error) {
	size, err := r.ResponseWriter.Write(b)
	r.responseData.size += size
	return size, err
}

func (r *loggingResponseWriter) WriteHeader(statusCode int) {
	r.ResponseWriter.WriteHeader(statusCode)
	r.responseData.status = statusCode
}

func NewResponseData() *responseData {
	return &responseData{
		status: 0,
		size:   0,
	}
}

func NewResponseWriter(r *responseData, w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{
		ResponseWriter: w,
		responseData:   r,
	}
}
