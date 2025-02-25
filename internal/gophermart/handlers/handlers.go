package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/4aleksei/gmart/internal/gophermart/handlers/middleware/httpgzip"
	"github.com/4aleksei/gmart/internal/gophermart/handlers/middleware/httplogs"
	"github.com/4aleksei/gmart/internal/gophermart/service"

	"github.com/4aleksei/gmart/internal/common/logger"
	"github.com/4aleksei/gmart/internal/common/models"

	"github.com/4aleksei/gmart/internal/gophermart/config"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/jwtauth/v5"
	"go.uber.org/zap"

	"github.com/golang-jwt/jwt/v4"
)

type (
	HandlersServer struct {
		cfg       *config.Config
		Srv       *http.Server
		l         *logger.ZapLogger
		key       string
		s         *service.HandleService
		tokenAuth *jwtauth.JWTAuth
	}
)

const (
	textHTMLContent        string = "text/html"
	applicationJSONContent string = "application/json"
	textPlainContent       string = "text/plain"

	textPlainContentCharset string = "text/plain; charset=utf-8"

	defaultHTTPshutdown int = 10
)

var (
	ErrAuthenticationFailed = errors.New("authentication_failed")
)

func NewHTTPServer(cfg *config.Config, l *logger.ZapLogger, s *service.HandleService) *HandlersServer {
	h := &HandlersServer{
		cfg:       cfg,
		key:       cfg.Key,
		l:         l,
		s:         s,
		tokenAuth: jwtauth.New("HS256", []byte(cfg.Key), nil),
	}

	h.Srv = &http.Server{
		Addr:              cfg.Address,
		Handler:           h.newRouter(),
		ReadHeaderTimeout: 2 * time.Second,
	}
	return h
}

func (h *HandlersServer) Start(ctx context.Context) error {
	go func() {
		h.l.Logger.Info("Starting server", zap.String("address", h.cfg.Address))
		if err := h.Srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			h.l.Logger.Debug("HTTP server error: ", zap.Error(err))
		}
		h.l.Logger.Info("Stopped serving new connections.")
	}()
	return nil
}

func (h *HandlersServer) Stop(ctx context.Context) error {
	h.l.Logger.Info("Server is shutting down...")
	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), time.Duration(defaultHTTPshutdown)*time.Second)
	defer shutdownRelease()

	if err := h.Srv.Shutdown(shutdownCtx); err != nil {
		h.l.Logger.Error("HTTP shutdown error :", zap.Error(err))
	} else {
		h.l.Logger.Info("Server shutdown complete")
	}
	return nil
}

func (h *HandlersServer) withLogging(next http.Handler) http.Handler {
	logFn := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		responseData := httplogs.NewResponseData()
		lw := httplogs.NewResponseWriter(responseData, w)

		next.ServeHTTP(lw, r)
		duration := time.Since(start)
		h.l.Logger.Info("got incoming HTTP request",
			zap.String("uri", r.RequestURI),
			zap.String("method", r.Method),
			zap.String("AcceptEnc", r.Header.Get("Accept-Encoding")),
			zap.String("ContentEnc", r.Header.Get("Content-Encoding")),
			zap.String("Accept", r.Header.Get("Accept")),
			zap.String("ContentType", r.Header.Get("Content-Type")),
			zap.Duration("duration", duration),
			zap.Int("resp_status", responseData.GetStatus()),
			zap.Int("resp_size", responseData.GetSize()))
	}
	return http.HandlerFunc(logFn)
}

func (h *HandlersServer) Serve() {
	go func() {
		if err := h.Srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			h.l.Logger.Debug("HTTP server error: ", zap.Error(err))
		}
		h.l.Logger.Info("Stopped serving new connections.")
	}()
}

func (h *HandlersServer) gzipMiddleware(next http.Handler) http.Handler {
	gzipfn := func(w http.ResponseWriter, r *http.Request) {
		ow := w
		acceptEncoding := r.Header.Get("Accept-Encoding")
		supportsGzip := strings.Contains(acceptEncoding, "gzip")
		if supportsGzip {
			cw := httpgzip.NewCompressWriter(w)
			ow = cw
			defer cw.Close()
		}
		contentEncoding := r.Header.Get("Content-Encoding")
		sendsGzip := strings.Contains(contentEncoding, "gzip")
		if sendsGzip {
			cr, err := httpgzip.NewCompressReader(r.Body)
			if err != nil {
				h.l.Logger.Debug("cannot decode gzip", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			r.Body = cr
			defer cr.Close()
		}
		next.ServeHTTP(ow, r)
	}
	return http.HandlerFunc(gzipfn)
}

func (h *HandlersServer) newRouter() http.Handler {
	mux := chi.NewRouter()
	mux.Use(h.withLogging)
	mux.Use(h.gzipMiddleware)

	mux.Group(func(r chi.Router) {
		r.Use(jwtauth.Verifier(h.tokenAuth))

		r.Use(jwtauth.Authenticator(h.tokenAuth))
		r.Use(middleware.Recoverer)

		r.Post("/api/user/orders", h.mainPagePostOrder)
		r.Get("/api/user/orders", h.mainPageGetOrders)

		r.Get("/api/user/withdrawals", h.mainPageGetWithdrawals)

		r.Get("/api/user/balance", h.mainPageGetBalance)
		r.Post("/api/user/balance/withdraw", h.mainPagePostWithdraw)
	})

	mux.Group(func(r chi.Router) {
		r.Use(middleware.Recoverer)
		r.Get("/", h.mainPage)
		r.Post("/api/user/register", h.mainPageRegister)
		r.Post("/api/user/login", h.mainPageLogin)
	})
	return mux
}

func (h *HandlersServer) testToken(req *http.Request) (string, error) {
	jwt, claims, _ := jwtauth.FromContext(req.Context())
	if jwt == nil || claims["sub"] == "" {
		return "", ErrAuthenticationFailed
	}
	return claims["sub"].(string), nil
}

func (h *HandlersServer) mainPagePostWithdraw(res http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Content-Type") != applicationJSONContent {
		http.Error(res, "Bad content type", http.StatusBadRequest)
		return
	}

	userID, err := h.testToken(req)
	if err != nil {
		http.Error(res, "Unauthorized access!", http.StatusUnauthorized)
		return
	}

	var withdraw models.Withdraw
	if err := withdraw.FromJSON(req.Body); err != nil {
		h.l.Logger.Debug("cannot decode request JSON body", zap.Error(err))
		res.WriteHeader(http.StatusInternalServerError)
		return
	} else {
		h.l.Logger.Debug("try order withdraw", zap.String("user", userID), zap.Any("order", withdraw))
	}

	err = h.s.PostWithdraw(req.Context(), userID, withdraw)

	if err != nil {
		if errors.Is(err, service.ErrBadValue) {
			h.l.Logger.Debug("order num error: ", zap.Error(err))
			res.WriteHeader(http.StatusUnprocessableEntity)
		} else if errors.Is(err, service.ErrBalanceNotEnough) {
			h.l.Logger.Debug("Error balance not enough", zap.Error(err))
			res.WriteHeader(http.StatusPaymentRequired)
		} else {
			h.l.Logger.Debug("Error registering withdraw", zap.Error(err))
			res.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	switch req.Header.Get("Accept") {
	case textHTMLContent:
		res.Header().Add("Content-Type", textHTMLContent)
	case applicationJSONContent:
		res.Header().Add("Content-Type", applicationJSONContent)
	default:
		res.Header().Add("Content-Type", textPlainContentCharset)
	}
	res.WriteHeader(http.StatusOK)
}

func (h *HandlersServer) mainPagePostOrder(res http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Content-Type") != textPlainContent {
		http.Error(res, "Bad content type", http.StatusBadRequest)
		return
	}

	userID, err := h.testToken(req)
	if err != nil {
		http.Error(res, "Unauthorized access!", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(req.Body)

	if err != nil {
		h.l.Logger.Debug("Read body", zap.Error(err))
		http.Error(res, "Error reading request body", http.StatusInternalServerError)
		return
	}

	switch req.Header.Get("Accept") {
	case textHTMLContent:
		res.Header().Add("Content-Type", textHTMLContent)
	case applicationJSONContent:
		res.Header().Add("Content-Type", applicationJSONContent)
	default:
		res.Header().Add("Content-Type", textPlainContentCharset)
	}

	err = h.s.RegisterOrder(req.Context(), userID, string(body))
	if err != nil {
		if errors.Is(err, service.ErrOrderAlreadyLoadedOtherUser) {
			h.l.Logger.Debug("Order load other user: ", zap.Error(err))
			res.WriteHeader(http.StatusConflict)
		} else if errors.Is(err, service.ErrBadValue) {
			h.l.Logger.Debug("order num error: ", zap.Error(err))
			res.WriteHeader(http.StatusUnprocessableEntity)
		} else if errors.Is(err, service.ErrOrderAlreadyLoaded) {
			h.l.Logger.Debug("order already loaded: ", zap.Error(err))
			res.WriteHeader(http.StatusOK)
		} else {
			h.l.Logger.Debug("Error registering order", zap.Error(err))
			res.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	res.WriteHeader(http.StatusAccepted)
}

func (h *HandlersServer) mainPageGetBalance(res http.ResponseWriter, req *http.Request) {
	userID, err := h.testToken(req)
	if err != nil {
		http.Error(res, "Unauthorized access!", http.StatusUnauthorized)
		return
	}

	res.Header().Add("Content-Type", applicationJSONContent)
	val, err := h.s.GetBalance(req.Context(), userID)
	if err != nil {
		h.l.Logger.Debug("get balance", zap.Error(err))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.l.Logger.Debug("handler get balance", zap.String("user", userID), zap.Any("balance", val))
	var buf bytes.Buffer
	if errson := models.JSONSEncodeBytes(io.Writer(&buf), val); errson != nil {
		h.l.Logger.Debug("error encoding response", zap.Error(errson))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	res.WriteHeader(http.StatusOK)

	if _, err := io.WriteString(res, buf.String()); err != nil {
		h.l.Logger.Debug("error writing response", zap.Error(err))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (h *HandlersServer) mainPageGetWithdrawals(res http.ResponseWriter, req *http.Request) {
	userID, err := h.testToken(req)
	if err != nil {
		http.Error(res, "Unauthorized access!", http.StatusUnauthorized)
		return
	}

	res.Header().Add("Content-Type", applicationJSONContent)

	val, err := h.s.GetWithdrawals(req.Context(), userID)
	if err != nil {
		h.l.Logger.Debug("get withdrawals", zap.Error(err))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(val) == 0 {
		h.l.Logger.Debug("no row for user  withdrawals")
		res.WriteHeader(http.StatusNoContent)
		return
	}

	var buf bytes.Buffer
	if errson := models.JSONSEncodeBytes(io.Writer(&buf), val); errson != nil {
		h.l.Logger.Debug("error encoding response", zap.Error(errson))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	res.WriteHeader(http.StatusOK)

	if _, err := io.WriteString(res, buf.String()); err != nil {
		h.l.Logger.Debug("error writing response", zap.Error(err))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (h *HandlersServer) mainPageGetOrders(res http.ResponseWriter, req *http.Request) {
	userID, err := h.testToken(req)
	if err != nil {
		http.Error(res, "Unauthorized access!", http.StatusUnauthorized)
		return
	}

	val, err := h.s.GetOrders(req.Context(), userID)
	if err != nil {
		h.l.Logger.Debug("get orders", zap.Error(err))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	res.Header().Add("Content-Type", applicationJSONContent)

	if len(val) == 0 {
		h.l.Logger.Debug("no row for user orders")
		res.WriteHeader(http.StatusNoContent)
		return
	}

	var buf bytes.Buffer
	if errson := models.JSONSEncodeBytes(io.Writer(&buf), val); errson != nil {
		h.l.Logger.Debug("error encoding response", zap.Error(errson))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	res.WriteHeader(http.StatusOK)

	if _, err := io.WriteString(res, buf.String()); err != nil {
		h.l.Logger.Debug("error writing response", zap.Error(err))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (h *HandlersServer) createToken(usernameID, name string) (string, error) {
	claims := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  usernameID,                       // Subject (user identifier)
		"name": name,                             // Subject name
		"iss":  "gophermart",                     // Issuer
		"exp":  time.Now().Add(time.Hour).Unix(), // Expiration time
		"iat":  time.Now().Unix(),                // Issued at
	})

	tokenString, err := claims.SignedString([]byte(h.key))
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

func (h *HandlersServer) mainPageRegister(res http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Content-Type") != applicationJSONContent {
		http.Error(res, "Bad content type", http.StatusBadRequest)
		return
	}

	var user models.UserRegistration
	if err := user.FromJSON(req.Body); err != nil {
		h.l.Logger.Debug("cannot decode request JSON body", zap.Error(err))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	userid, err := h.s.RegisterUser(req.Context(), user)
	if err != nil {
		if errors.Is(err, service.ErrAuthenticationFailed) {
			h.l.Logger.Debug("User already exists: ", zap.Error(err))
			res.WriteHeader(http.StatusConflict)
		} else if errors.Is(err, service.ErrBadPass) {
			h.l.Logger.Debug("name or pass error: ", zap.Error(err))
			res.WriteHeader(http.StatusBadRequest)
		} else {
			h.l.Logger.Debug("RegisterUser: ", zap.Error(err))
			res.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	token, err := h.createToken(userid, user.Name)

	if err != nil {
		h.l.Logger.Error("Error creating token", zap.Error(err))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch req.Header.Get("Accept") {
	case textHTMLContent:
		res.Header().Add("Content-Type", textHTMLContent)
	case applicationJSONContent:
		res.Header().Add("Content-Type", applicationJSONContent)
	default:
		res.Header().Add("Content-Type", textPlainContentCharset)
	}

	http.SetCookie(res, &http.Cookie{
		HttpOnly: true,
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		SameSite: http.SameSiteLaxMode,
		// Uncomment below for HTTPS:
		// Secure: true,
		Name:  "jwt", // Must be named "jwt" or else the token cannot be searched for by jwtauth.Verifier.
		Value: token,
	})
	res.WriteHeader(http.StatusOK)
}

func (h *HandlersServer) mainPageLogin(res http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Content-Type") != applicationJSONContent {
		http.Error(res, "Bad content type", http.StatusBadRequest)
		return
	}

	var user models.UserRegistration
	if err := user.FromJSON(req.Body); err != nil {
		h.l.Logger.Debug("cannot decode request JSON body", zap.Error(err))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	userid, err := h.s.LoginUser(req.Context(), user)
	if err != nil {
		if errors.Is(err, service.ErrAuthenticationFailed) {
			h.l.Logger.Debug("ErrAuthentication : ", zap.Error(err))
			res.WriteHeader(http.StatusUnauthorized)
		} else if errors.Is(err, service.ErrBadPass) {
			h.l.Logger.Debug("name or pass error: ", zap.Error(err))
			res.WriteHeader(http.StatusBadRequest)
		} else {
			h.l.Logger.Debug("LoginUser: ", zap.Error(err))
			res.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	switch req.Header.Get("Accept") {
	case textHTMLContent:
		res.Header().Add("Content-Type", textHTMLContent)
	case applicationJSONContent:
		res.Header().Add("Content-Type", applicationJSONContent)
	default:
		res.Header().Add("Content-Type", textPlainContentCharset)
	}

	token, err := h.createToken(userid, user.Name)

	if err != nil {
		h.l.Logger.Error("Error creating token", zap.Error(err))
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.SetCookie(res, &http.Cookie{
		HttpOnly: true,
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		SameSite: http.SameSiteLaxMode,
		// Uncomment below for HTTPS:
		// Secure: true,
		Name:  "jwt", // Must be named "jwt" or else the token cannot be searched for by jwtauth.Verifier.
		Value: token,
	})
	res.WriteHeader(http.StatusOK)
}

func (h *HandlersServer) mainPage(res http.ResponseWriter, req *http.Request) {
	if req.URL.String() == "" || req.URL.String() == "/" {
		val := "Server Started"
		switch req.Header.Get("Accept") {
		case textHTMLContent:
			res.Header().Add("Content-Type", textHTMLContent)
		case applicationJSONContent:
			res.Header().Add("Content-Type", applicationJSONContent)
		default:
			res.Header().Add("Content-Type", textPlainContentCharset)
		}
		res.WriteHeader(http.StatusOK)
		if _, err := res.Write([]byte(val)); err != nil {
			h.l.Logger.Debug("error writing response", zap.Error(err))
			res.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(res, "Bad request", http.StatusBadRequest)
	}
}
