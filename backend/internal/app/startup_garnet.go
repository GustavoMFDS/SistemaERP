package app

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/redis/go-redis/v9"
)

// go-redis's default TLS dialer does not pass the request context into
// tls.DialWithDialer. Preserve the configured certificate, hostname, TLS
// policy and dial timeout, but propagate cancellation for local Windows
// startup and subsequent requests. Never replace an explicit custom dialer.
func configureLocalRedisStartupDialer(opts *redis.Options) {
	if opts.TLSConfig == nil || opts.Dialer != nil {
		return
	}
	timeout := opts.DialTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: timeout, KeepAlive: 5 * time.Minute},
		Config:    opts.TLSConfig,
	}
	opts.Dialer = dialer.DialContext
}

// Only fixed categories are logged; no raw errors, addresses, credentials,
// certificate names or Redis responses enter this diagnostic.
func redisStartupPingFailureKind(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}
	var cert *tls.CertificateVerificationError
	if errors.As(err, &cert) {
		return "certificate-validation"
	}
	var response redis.Error
	if errors.As(err, &response) {
		return "redis-response"
	}
	var header tls.RecordHeaderError
	if errors.As(err, &header) {
		return "tls-protocol"
	}
	if startupPingTransient(err, false) {
		return "transport-transient"
	}
	return "other"
}
