package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"time"

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

	"github.com/go-chi/jwtauth/v5"

	"github.com/greatcloak/decimal"
)

const (
	defaultKeyLen int = 16
)

func testRequest(t *testing.T, ts *httptest.Server, method,
	path string, body string, contentType string, contentEnc string, jwt []*http.Cookie) (*http.Response, string) {
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

	if jwt != nil {
		if len(jwt) > 0 {
			for _, v := range jwt {
				req.AddCookie(v)
			}
		}
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
			resp, respBody := testRequest(t, ts, tt.req.method, tt.req.url, tt.req.body, tt.req.contentType, tt.req.contentEnc, nil)
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
	cfg := &config.Config{}
	if cfg.Key == "" {
		b, err := utils.GenerateRandom(defaultKeyLen)
		if err != nil {
			panic("no key")
		}
		cfg.Key = hex.EncodeToString(b)
	}
	if cfg.KeySignature == "" {
		b, err := utils.GenerateRandom(defaultKeyLen)
		if err != nil {
			panic("no key")
		}
		cfg.KeySignature = hex.EncodeToString(b)
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
	h.key = cfg.Key
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
			resp, respBody := testRequest(t, ts, tt.req.method, tt.req.url, tt.req.body, tt.req.contentType, tt.req.contentEnc, nil)
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

func Test_handlers_mainPagePostOrder(t *testing.T) {
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

	stor.EXPECT().
		InsertOrder(gomock.Any(), gomock.Any()).
		Return(nil).
		MaxTimes(5)

	serV := service.NewService(stor, cfg, nil)

	h := new(HandlersServer)
	h.s = serV
	h.key = cfg.Key
	h.tokenAuth = jwtauth.New("HS256", []byte(cfg.Key), nil)
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
		{name: "Post Order before login No1", req: request{method: http.MethodPost, url: "/api/user/orders", body: "5062821234567892", contentType: "text/plain"}, want: want{statusCode: http.StatusUnauthorized, contentType: "", body: ""}},
		{name: "Login User  No2", req: request{method: http.MethodPost, url: "/api/user/login", body: " {\"login\":\"" + name + "\" , \"password\":\"" + passWord + "\" }  ", contentType: "application/json"}, want: want{statusCode: http.StatusOK, contentType: "", body: ""}},
		{name: "Post Order No3", req: request{method: http.MethodPost, url: "/api/user/orders", body: "5062821234567892", contentType: "text/plain"}, want: want{statusCode: http.StatusAccepted, contentType: "", body: ""}},
		{name: "Post Order with Non Luhn Number No4", req: request{method: http.MethodPost, url: "/api/user/orders", body: "123456", contentType: "text/plain"}, want: want{statusCode: http.StatusUnprocessableEntity, contentType: "", body: ""}},
	}

	jwt := make([]*http.Cookie, 0)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, respBody := testRequest(t, ts, tt.req.method, tt.req.url, tt.req.body, tt.req.contentType, tt.req.contentEnc, jwt)
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

			if len(jwt) == 0 {
				jwt = append(jwt, resp.Cookies()...)
			}
			resp.Body.Close()
		})
	}
}

func Test_handlers_mainPageGetOrder(t *testing.T) {
	decimal.MarshalJSONWithoutQuotes = true
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
	timeNow := time.Now()
	strTime := timeNow.Format(time.RFC3339Nano) //RFC1123
	var orders = []store.Order{
		{
			OrderID: 5062821234567892,
			UserID:  1,
			Status:  "PROCESSED",
			Accrual: decimal.RequireFromString("500"),
			TimeU:   timeNow,
			TimeC:   timeNow,
		},
		{
			OrderID: 5062821234567893,
			UserID:  1,
			Status:  "INVALID",
			Accrual: decimal.RequireFromString("0"),
			TimeU:   timeNow,
			TimeC:   timeNow,
		}}

	stor.EXPECT().
		GetUser(gomock.Any(), arg).
		Return(argRet, nil).
		MaxTimes(5)

	stor.EXPECT().
		GetUser(gomock.Any(), argWrong).
		Return(argRet, pg.ErrRowNotFound).
		MaxTimes(5)

	stor.EXPECT().
		GetOrders(gomock.Any(), gomock.Any()).
		Return(orders, nil).
		MaxTimes(5)

	serV := service.NewService(stor, cfg, nil)

	h := new(HandlersServer)
	h.s = serV
	h.key = cfg.Key
	h.tokenAuth = jwtauth.New("HS256", []byte(cfg.Key), nil)
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
		{name: "Get Orders before login No1", req: request{method: http.MethodGet, url: "/api/user/orders", body: "5062821234567892", contentType: "text/plain"}, want: want{statusCode: http.StatusUnauthorized, contentType: "", body: ""}},
		{name: "Login User  No2", req: request{method: http.MethodPost, url: "/api/user/login", body: " {\"login\":\"" + name + "\" , \"password\":\"" + passWord + "\" }  ", contentType: "application/json"}, want: want{statusCode: http.StatusOK, contentType: "", body: ""}},
		{name: "Get Orders No3", req: request{method: http.MethodGet, url: "/api/user/orders", body: "", contentType: "text/plain"}, want: want{statusCode: http.StatusOK, contentType: "application/json",
			body: "[{\"number\": \"5062821234567892\", \"status\": \"PROCESSED\", \"accrual\": 500, \"uploaded_at\": \"" + strTime + "\" }, {\"number\": \"5062821234567893\", \"status\": \"INVALID\", \"accrual\": 0, \"uploaded_at\": \"" + strTime + "\" }]"},
		},
	}

	jwt := make([]*http.Cookie, 0)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, respBody := testRequest(t, ts, tt.req.method, tt.req.url, tt.req.body, tt.req.contentType, tt.req.contentEnc, jwt)
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

			if len(jwt) == 0 {
				jwt = append(jwt, resp.Cookies()...)
			}
			resp.Body.Close()
		})
	}
}

func Test_handlers_mainPageGetBalance(t *testing.T) {
	decimal.MarshalJSONWithoutQuotes = true

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

	timeNow := time.Now()
	var balance = store.Balance{
		UserID:    1,
		Accrual:   decimal.RequireFromString("500"),
		Withdrawn: decimal.RequireFromString("10"),
		TimeC:     timeNow,
	}

	stor.EXPECT().
		GetUser(gomock.Any(), arg).
		Return(argRet, nil).
		MaxTimes(5)

	stor.EXPECT().
		GetUser(gomock.Any(), argWrong).
		Return(argRet, pg.ErrRowNotFound).
		MaxTimes(5)

	stor.EXPECT().
		GetBalance(gomock.Any(), gomock.Any()).
		Return(balance, nil).
		MaxTimes(5)

	serV := service.NewService(stor, cfg, nil)

	h := new(HandlersServer)
	h.s = serV
	h.key = cfg.Key
	h.tokenAuth = jwtauth.New("HS256", []byte(cfg.Key), nil)
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
		{name: "Get Balance before login No1", req: request{method: http.MethodGet, url: "/api/user/balance", body: "", contentType: "text/plain"}, want: want{statusCode: http.StatusUnauthorized, contentType: "", body: ""}},
		{name: "Login User  No2", req: request{method: http.MethodPost, url: "/api/user/login", body: " {\"login\":\"" + name + "\" , \"password\":\"" + passWord + "\" }  ", contentType: "application/json"}, want: want{statusCode: http.StatusOK, contentType: "", body: ""}},
		{name: "Get Balance No3", req: request{method: http.MethodGet, url: "/api/user/balance", body: "", contentType: "text/plain"}, want: want{statusCode: http.StatusOK, contentType: "application/json", body: "{\"current\": 500, \"withdrawn\": 10 }"}},
	}

	jwt := make([]*http.Cookie, 0)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, respBody := testRequest(t, ts, tt.req.method, tt.req.url, tt.req.body, tt.req.contentType, tt.req.contentEnc, jwt)
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

			if len(jwt) == 0 {
				jwt = append(jwt, resp.Cookies()...)
			}
			resp.Body.Close()
		})
	}
}

func Test_handlers_mainPagePostWithdraw(t *testing.T) {
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

	var withdraw = store.Withdraw{
		UserID:  1,
		OrderID: 2377225624,
		Sum:     decimal.RequireFromString("751"),
	}

	stor.EXPECT().
		GetUser(gomock.Any(), arg).
		Return(argRet, nil).
		MaxTimes(5)

	stor.EXPECT().
		GetUser(gomock.Any(), argWrong).
		Return(argRet, pg.ErrRowNotFound).
		MaxTimes(5)

	stor.EXPECT().
		InsertWithdraw(gomock.Any(), withdraw).
		Return(nil).
		MaxTimes(5)

	serV := service.NewService(stor, cfg, nil)

	h := new(HandlersServer)
	h.s = serV
	h.key = cfg.Key
	h.tokenAuth = jwtauth.New("HS256", []byte(cfg.Key), nil)
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
		{name: "Set Order withdraw before login No1", req: request{method: http.MethodPost, url: "/api/user/balance/withdraw", body: "", contentType: "text/plain"}, want: want{statusCode: http.StatusUnauthorized, contentType: "", body: ""}},
		{name: "Login User  No2", req: request{method: http.MethodPost, url: "/api/user/login", body: " {\"login\":\"" + name + "\" , \"password\":\"" + passWord + "\" }  ", contentType: "application/json"}, want: want{statusCode: http.StatusOK, contentType: "", body: ""}},
		{name: "Set Order withdraw No3", req: request{method: http.MethodPost, url: "/api/user/balance/withdraw", body: "{\"order\": \"2377225624\",  \"sum\": 751  }", contentType: "application/json"}, want: want{statusCode: http.StatusOK, contentType: "", body: ""}},
	}

	jwt := make([]*http.Cookie, 0)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, respBody := testRequest(t, ts, tt.req.method, tt.req.url, tt.req.body, tt.req.contentType, tt.req.contentEnc, jwt)
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

			if len(jwt) == 0 {
				jwt = append(jwt, resp.Cookies()...)
			}
			resp.Body.Close()
		})
	}
}

func Test_handlers_mainPageGetWithdrawals(t *testing.T) {
	decimal.MarshalJSONWithoutQuotes = true
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

	timeNow := time.Now()
	strTime := timeNow.Format(time.RFC3339Nano) //RFC1123
	var withdraw = []store.Withdraw{
		{
			UserID:  1,
			OrderID: 2377225624,
			Sum:     decimal.RequireFromString("751"),
			TimeC:   timeNow,
		},
		{
			UserID:  1,
			OrderID: 2377225625,
			Sum:     decimal.RequireFromString("200"),
			TimeC:   timeNow,
		},
	}
	decimal.MarshalJSONWithoutQuotes = true

	stor.EXPECT().
		GetUser(gomock.Any(), arg).
		Return(argRet, nil).
		MaxTimes(5)

	stor.EXPECT().
		GetUser(gomock.Any(), argWrong).
		Return(argRet, pg.ErrRowNotFound).
		MaxTimes(5)

	stor.EXPECT().
		GetWithdrawals(gomock.Any(), gomock.Any()).
		Return(withdraw, nil).
		MaxTimes(5)

	serV := service.NewService(stor, cfg, nil)

	h := new(HandlersServer)
	h.s = serV
	h.key = cfg.Key
	h.tokenAuth = jwtauth.New("HS256", []byte(cfg.Key), nil)
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
		{name: "Get withdrawals before login No1", req: request{method: http.MethodGet, url: "/api/user/withdrawals", body: "", contentType: "text/plain"}, want: want{statusCode: http.StatusUnauthorized, contentType: "", body: ""}},
		{name: "Login User  No2", req: request{method: http.MethodPost, url: "/api/user/login", body: " {\"login\":\"" + name + "\" , \"password\":\"" + passWord + "\" }  ", contentType: "application/json"}, want: want{statusCode: http.StatusOK, contentType: "", body: ""}},
		{name: "Set Order withdraw No3", req: request{method: http.MethodGet, url: "/api/user/withdrawals", body: "", contentType: "text/plain"}, want: want{statusCode: http.StatusOK, contentType: "application/json",
			body: "[{\"order\": \"2377225624\", \"sum\": 751, \"processed_at\": \"" + strTime + "\" }, {\"order\": \"2377225625\", \"sum\": 200, \"processed_at\": \"" + strTime + "\" }]"},
		},
	}

	jwt := make([]*http.Cookie, 0)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, respBody := testRequest(t, ts, tt.req.method, tt.req.url, tt.req.body, tt.req.contentType, tt.req.contentEnc, jwt)
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

			if len(jwt) == 0 {
				jwt = append(jwt, resp.Cookies()...)
			}
			resp.Body.Close()
		})
	}
}
