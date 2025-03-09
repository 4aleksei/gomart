package hmacsha256

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"hash"
	"io"
)

type HmacWriter struct {
	w io.Writer
	h hash.Hash
}

func NewWriter(w io.Writer, key []byte) *HmacWriter {
	h := hmac.New(sha256.New, key)
	return &HmacWriter{
		w: w,
		h: h,
	}
}

func (h *HmacWriter) Write(p []byte) (int, error) {
	h.h.Write(p)
	return h.w.Write(p)
}

func (h *HmacWriter) GetSig() []byte {
	return h.h.Sum(nil)
}

type HmacReader struct {
	r  io.ReadCloser
	hr hash.Hash
}

func NewReader(r io.ReadCloser, key []byte) *HmacReader {
	h := hmac.New(sha256.New, key)
	return &HmacReader{
		r:  r,
		hr: h,
	}
}

func (h *HmacReader) Read(p []byte) (n int, err error) {
	n, err = h.r.Read(p)
	if n > 0 {
		h.hr.Write(p[:n])
	}
	return
}

func (h *HmacReader) Close() error {
	return h.r.Close()
}

func (h *HmacReader) GetSig() []byte {
	return h.hr.Sum(nil)
}

var (
	ErrBadReaderType = errors.New("invalid type of Reader")
)

func GetSig(r io.ReadCloser) ([]byte, error) {
	if m, ok := r.(*HmacReader); ok {
		return m.GetSig(), nil
	}
	return nil, ErrBadReaderType
}
