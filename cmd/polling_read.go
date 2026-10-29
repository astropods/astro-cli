package cmd

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"
)

func pollingGET(ctx context.Context, requestURL, token string, verbose bool, dest any) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	const attempts = 4
	for attempt := 0; ; attempt++ {
		status, err := apiCall(ctx, http.MethodGet, requestURL, nil, token, verbose, dest)
		if ctx.Err() != nil {
			return status, ctx.Err()
		}
		if err == nil || attempt == attempts-1 || !retryableRead(status, err) {
			return status, err
		}
		delay := 250 * time.Millisecond << attempt
		delay += time.Duration(rand.Int64N(int64(delay))) //nolint:gosec
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			delay = max(delay, retryAfterDelay(apiErr.RetryAfter, time.Now()))
		}
		if err := waitWithContext(ctx, delay); err != nil {
			return status, err
		}
	}
}

func retryableRead(status int, err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if status >= 300 {
		switch status {
		case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError,
			http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	var networkErr net.Error
	return errors.As(err, &networkErr) && networkErr.Timeout() ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE)
}

func retryAfterDelay(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(0, date.Sub(now))
	}
	return 0
}

func waitWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
