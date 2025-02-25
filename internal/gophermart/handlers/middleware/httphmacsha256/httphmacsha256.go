package httphmacsha256

import (
	"encoding/hex"
	"errors"
	"io"
	"net/http"

	"github.com/4aleksei/gmart/internal/common/middleware/hmacsha256"
)

type httphmacsha256Writer struct {
	w  http.ResponseWriter
	hw *hmacsha256.HmacWriter
}

func NewWriter(w http.ResponseWriter, key []byte) *httphmacsha256Writer {
	return &httphmacsha256Writer{
		w:  w,
		hw: hmacsha256.NewWriter(w, key),
	}
}

func (h *httphmacsha256Writer) Header() http.Header {
	return h.w.Header()
}

func (h *httphmacsha256Writer) Write(p []byte) (int, error) {
	return h.hw.Write(p)
}

const unsuccessStatusCode int = 300

func (h *httphmacsha256Writer) WriteHeader(statusCode int) {
	if statusCode < unsuccessStatusCode {
		h.w.Header().Set("HashSHA256", hex.EncodeToString(h.hw.GetSig()))
	}
	h.w.WriteHeader(statusCode)
}

type httphmacsha256Reader struct {
	hr *hmacsha256.HmacReader
}

func NewReader(r io.ReadCloser, key []byte) *httphmacsha256Reader {
	return &httphmacsha256Reader{
		hr: hmacsha256.NewReader(r, key),
	}
}

func (h *httphmacsha256Reader) Read(p []byte) (n int, err error) {
	return h.hr.Read(p)
}

func (h *httphmacsha256Reader) Close() error {
	return h.hr.Close()
}

func (h *httphmacsha256Reader) GetSig() []byte {
	return h.hr.GetSig()
}

var (
	ErrBadReaderType = errors.New("invalid type of Reader")
)

func GetSig(r io.ReadCloser) ([]byte, error) {
	if m, ok := r.(*httphmacsha256Reader); ok {
		return m.GetSig(), nil
	}
	return nil, ErrBadReaderType
}
