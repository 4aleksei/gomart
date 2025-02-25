package utils

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"sync"
	"time"
)

type WaitGroupTimeout struct {
	sync.WaitGroup
}

var (
	ErrWgWaitTimeOut = errors.New("wait timeout")
)

func (wg *WaitGroupTimeout) WaitWithTimeout(ctx context.Context, timeout time.Duration) error {
	timeoutChan := time.After(timeout)
	waitChan := make(chan struct{})

	go func() {
		wg.Wait()
		close(waitChan)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timeoutChan:
		return ErrWgWaitTimeOut
	case <-waitChan:
		return nil
	}
}

func SleepCancellable(ctx context.Context, t time.Duration) {
	sleep, cancel := context.WithTimeout(ctx, t)
	defer cancel()
	<-sleep.Done()
}

func Setint64(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

func Setfloat64(f *float64) float64 {
	if f == nil {
		return 0.0
	}
	return *f
}

func probeDefault(err error) bool {
	return true
}

func RetryTimes() []int {
	return []int{1000, 3000, 5000}
}

func RetryAction(
	ctx context.Context,
	timers []int,
	callback func(ctx context.Context) error,
	probers ...func(err error) bool,
) error {
	var err error
	if len(probers) == 0 {
		probers = append(probers, probeDefault)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:

			err = callback(ctx)

			if err != nil {
				shouldContinue := false

				for i := 0; i < len(probers); i++ {
					prober := probers[i]

					if prober(err) {
						shouldContinue = true
					}
				}

				if shouldContinue && (len(timers) > 0) {
					SleepCancellable(ctx, time.Duration(timers[0])*time.Millisecond)
					timers = timers[1:]

					continue
				}

				return err
			}

			return nil
		}
	}
}

const (
	moduleDef    uint64 = 10
	moduleDefMin uint64 = 9
)

func ValidLuhn(number uint64) bool {
	return (number%10+checksumLuhn(number/moduleDef))%moduleDef == 0
}

func checksumLuhn(number uint64) uint64 {
	var luhn uint64

	for i := 0; number > 0; i++ {
		cur := number % moduleDef

		if i%2 == 0 { // even
			cur *= 2
			if cur > moduleDefMin {
				cur = cur%moduleDef + cur/moduleDef
			}
		}

		luhn += cur
		number /= moduleDef
	}
	return luhn % moduleDef
}

func GenerateRandom(size int) ([]byte, error) {
	b := make([]byte, size)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func HashPass(p []byte, k string) []byte {
	h := hmac.New(sha256.New, []byte(k))
	dst := h.Sum(p)
	return dst
}
