package httpclientpool

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"

	"net"
	"time"

	"github.com/4aleksei/gmart/internal/common/httpclientpool/job"
	"github.com/4aleksei/gmart/internal/common/logger"

	"strconv"

	"github.com/4aleksei/gmart/internal/common/models"
	"go.uber.org/zap"
)

const (
	textHTMLContent        string = "text/html"
	textPlainContent       string = "text/plain"
	applicationJSONContent string = "application/json"
	gzipContent            string = "gzip"
)

const (
	HTTPRetryCode   int = 429
	HTTPSuccessCode int = 200
)

var (
	ErrReadDone   = errors.New("read done")
	ErrChanClosed = errors.New("closed chan")

	ErrJSONDecode = errors.New("cannot decode resp JSON body")
	ErrBadValue   = errors.New("orderID bad value")
)

type (
	Config struct {
		RateLimit uint64
		Address   string
	}

	PoolHandler struct {
		WorkerCount int
		clients     []clientInstance
		cfg         Config
		l           *logger.ZapLogger
	}
	functioExec func(context.Context, *sync.WaitGroup, *http.Client,
		<-chan job.Job, chan<- job.Result, *Config, *logger.ZapLogger)
	clientInstance struct {
		execFn functioExec
		client *http.Client
		cfg    *Config
	}

	resulAccrual struct {
		status   int
		waitTime int
		value    models.OrderAccrual
		err      error
	}
)

func NewHandler(l *logger.ZapLogger) *PoolHandler {
	return &PoolHandler{l: l}
}

func (p *PoolHandler) SetCfgInit(r uint64, a string) {
	p.cfg = Config{
		RateLimit: r,
		Address:   a,
	}
	p.WorkerCount = int(r)
	p.clients = make([]clientInstance, p.WorkerCount)
	for i := 0; i < p.WorkerCount; i++ {
		p.clients[i] = *newClientInstance(&p.cfg)
	}
}

func newClientInstance(cfg *Config) *clientInstance {
	return &clientInstance{
		execFn: workerPlain,
		client: newClient(),
		cfg:    cfg,
	}
}

func newClient() *http.Client {
	var netTransport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 2 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 2 * time.Second,
	}
	return &http.Client{
		Transport: netTransport,
	}
}

func workerPlain(ctx context.Context, wg *sync.WaitGroup, client *http.Client,
	jobs <-chan job.Job, results chan<- job.Result, cfg *Config, l *logger.ZapLogger) {
	defer wg.Done()
	server := cfg.Address + "/api/orders/"
	for j := range jobs {
		select {
		case <-ctx.Done():
			return
		default:
			data := strconv.FormatUint(j.Value.OrderID, 10)
			resClient, err := plainTxtFunc(ctx, client, server, data, l)
			if err != nil && errors.Is(err, context.Canceled) {
				return
			}

			var res = job.Result{
				Value: j.Value,
				Err:   err,
				ID:    j.ID,
			}

			if resClient != nil {
				res.Result = resClient.status
				res.WaitSec = resClient.waitTime
				res.Value.Accrual = resClient.value.Accrual
				res.Value.Status = resClient.value.Status
			}
			results <- res
		}
	}
}

func (p *PoolHandler) StartPool(ctx context.Context, jobs chan job.Job, results chan job.Result, wg *sync.WaitGroup) {
	for i := 0; i < p.WorkerCount; i++ {
		wg.Add(1)
		go p.clients[i].execFn(ctx, wg, p.clients[i].client, jobs, results, &p.cfg, p.l)
	}
}

func plainTxtFunc(ctx context.Context, client *http.Client, server, data string, l *logger.ZapLogger) (*resulAccrual, error) {
	res, err := newPGetReq(ctx, client, server+data, http.NoBody, l)
	return res, err
}

func newPGetReq(ctx context.Context, client *http.Client,
	server string, requestBody io.Reader, l *logger.ZapLogger) (*resulAccrual, error) {
	l.Logger.Debug("request ", zap.String("url ", server))

	req, err := http.NewRequestWithContext(ctx, "GET", server, requestBody)
	if err != nil {
		l.Logger.Debug("make req error ", zap.Error(err))
		return nil, err
	}
	req.Header.Set("Content-Type", textPlainContent)
	req.Header.Set("Accept", applicationJSONContent)

	resp, err := client.Do(req)
	if err != nil {
		l.Logger.Debug("DO req error ", zap.Error(err))
		return nil, err
	}
	defer resp.Body.Close()

	result := new(resulAccrual)
	result.status = resp.StatusCode

	l.Logger.Debug("status ", zap.Int("new status", resp.StatusCode))

	if resp.StatusCode == HTTPRetryCode {
		if resp.Header.Get("Retry-After") != "" {
			result.waitTime, _ = strconv.Atoi(resp.Header.Get("Retry-After"))
		}
	} else if resp.StatusCode == HTTPSuccessCode {
		err = result.value.FromJSON(resp.Body)
		if err != nil {
			result.err = ErrJSONDecode
		}
		l.Logger.Debug("result ", zap.Any("order", result.value))
	}
	return result, nil
}
