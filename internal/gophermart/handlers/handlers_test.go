package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/4aleksei/gmart/internal/common/logger"
	"github.com/4aleksei/gmart/internal/common/store"
	"github.com/4aleksei/gmart/internal/common/store/mock"
	"github.com/4aleksei/gmart/internal/common/store/pg"
	"github.com/4aleksei/gmart/internal/common/utils"
	"github.com/4aleksei/gmart/internal/gophermart/config"
	"github.com/4aleksei/gmart/internal/gophermart/service"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRequest(t *testing.T, ts *httptest.Server, method,
	path string, body string, contentType string, contentEnc string) (*http.Response, string) {
	var buf bytes.Buffer
	if contentEnc != "" {
		gz := gzip.NewWriter(&buf)
		_, err := gz.Write([]byte(body))
		require.NoError(t, err)
		gz.Flush()
	} else {
		buf.WriteString(body)
	}

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, method, ts.URL+path, &buf)
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Add("Content-Type", contentType)
	}
	if contentEnc != "" {
		req.Header.Add("Content-Encoding", contentEnc)
		req.Header.Add("Accept-Encoding", contentEnc)
	}
	require.NoError(t, err)
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, string(respBody)
}

func Test_handlers_mainPageRegister(t *testing.T) {
	type want struct {
		contentType string
		statusCode  int
		body        string
		contentEnc  string
	}
	type request struct {
		method      string
		url         string
		body        string
		contentType string
		contentEnc  string
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	stor := mock.NewMockStore(ctrl)
	cfg := &config.Config{
		Key:          "Test",
		KeySignature: "Test",
	}

	passWord := "12345"
	name := "Vasia"
	passWordSig := hex.EncodeToString(utils.HashPass([]byte(passWord), cfg.KeySignature))
	arg := store.User{
		Name:     name,
		Password: passWordSig,
	}
	argRet := store.User{
		Name:     name,
		Password: passWordSig,
		ID:       1,
	}

	stor.EXPECT().
		AddUser(gomock.Any(), arg).
		Return(argRet, nil)

	serV := service.NewService(stor, cfg, nil)

	h := new(HandlersServer)
	h.s = serV
	var errL error
	h.l, errL = logger.New(logger.Config{Level: "debug"})
	require.NoError(t, errL)

	ts := httptest.NewServer(h.newRouter())
	defer ts.Close()
	tests := []struct {
		name string
		req  request
		want want
	}{
		{name: "Register User Test No1", req: request{method: http.MethodPost, url: "/api/user/register", body: " {\"login\":\"" + name + "\" , \"password\":\"" + passWord + "\" }  ", contentType: "application/json"}, want: want{statusCode: http.StatusOK, contentType: "", body: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, respBody := testRequest(t, ts, tt.req.method, tt.req.url, tt.req.body, tt.req.contentType, tt.req.contentEnc)
			assert.Equal(t, tt.want.statusCode, resp.StatusCode)
			if tt.want.contentType != "" {
				assert.Equal(t, tt.want.contentType, resp.Header.Get("Content-Type"))
			}
			if tt.want.contentEnc != "" {
				assert.Equal(t, tt.want.contentEnc, resp.Header.Get("Content-Encoding"))

				body, err := gzip.NewReader(strings.NewReader(respBody))
				require.NoError(t, err)
				buf, errR := io.ReadAll(body)
				require.NoError(t, errR)
				if tt.want.body != "" {
					assert.JSONEq(t, tt.want.body, string(buf))
				}
			} else {
				if tt.want.body != "" {
					assert.JSONEq(t, tt.want.body, respBody)
				}
			}

			resp.Body.Close()
		})
	}
}

func Test_handlers_mainPageLogin(t *testing.T) {
	type want struct {
		contentType string
		statusCode  int
		body        string
		contentEnc  string
	}
	type request struct {
		method      string
		url         string
		body        string
		contentType string
		contentEnc  string
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	stor := mock.NewMockStore(ctrl)
	cfg := &config.Config{
		Key:          "Test",
		KeySignature: "Test",
	}

	passWord := "12345"
	name := "Vasia"
	wrongname := "WrongName"
	passWordSig := hex.EncodeToString(utils.HashPass([]byte(passWord), cfg.KeySignature))
	arg := store.User{
		Name: name,
	}
	argWrong := store.User{
		Name: wrongname,
	}
	argRet := store.User{
		Name:     name,
		Password: passWordSig,
		ID:       1,
	}

	stor.EXPECT().
		GetUser(gomock.Any(), arg).
		Return(argRet, nil).
		MaxTimes(5)

	stor.EXPECT().
		GetUser(gomock.Any(), argWrong).
		Return(argRet, pg.ErrRowNotFound).
		MaxTimes(5)

	serV := service.NewService(stor, cfg, nil)

	h := new(HandlersServer)
	h.s = serV
	var errL error
	h.l, errL = logger.New(logger.Config{Level: "debug"})
	require.NoError(t, errL)

	ts := httptest.NewServer(h.newRouter())
	defer ts.Close()
	tests := []struct {
		name string
		req  request
		want want
	}{
		{name: "Login User Test No1", req: request{method: http.MethodPost, url: "/api/user/login", body: " {\"login\":\"" + name + "\" , \"password\":\"" + passWord + "\" }  ", contentType: "application/json"}, want: want{statusCode: http.StatusOK, contentType: "", body: ""}},
		{name: "Login User Test No2", req: request{method: http.MethodPost, url: "/api/user/login", body: " {\"login\":\"" + name + "\" , \"password\":\"" + "wrongpass" + "\" }  ", contentType: "application/json"}, want: want{statusCode: http.StatusUnauthorized, contentType: "", body: ""}},
		{name: "Login User Test No3", req: request{method: http.MethodPost, url: "/api/user/login", body: " {\"login\":\"" + wrongname + "\" , \"password\":\"" + passWord + "\" }  ", contentType: "application/json"}, want: want{statusCode: http.StatusUnauthorized, contentType: "", body: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, respBody := testRequest(t, ts, tt.req.method, tt.req.url, tt.req.body, tt.req.contentType, tt.req.contentEnc)
			assert.Equal(t, tt.want.statusCode, resp.StatusCode)
			if tt.want.contentType != "" {
				assert.Equal(t, tt.want.contentType, resp.Header.Get("Content-Type"))
			}
			if tt.want.contentEnc != "" {
				assert.Equal(t, tt.want.contentEnc, resp.Header.Get("Content-Encoding"))

				body, err := gzip.NewReader(strings.NewReader(respBody))
				require.NoError(t, err)
				buf, errR := io.ReadAll(body)
				require.NoError(t, errR)
				if tt.want.body != "" {
					assert.JSONEq(t, tt.want.body, string(buf))
				}
			} else {
				if tt.want.body != "" {
					assert.JSONEq(t, tt.want.body, respBody)
				}
			}

			resp.Body.Close()
		})
	}
}
