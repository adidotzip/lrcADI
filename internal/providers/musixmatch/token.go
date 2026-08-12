package musixmatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/f1nniboy/lrcmux/internal/cache"
	"github.com/f1nniboy/lrcmux/internal/providers"
)

const tokenTTL = 0 // 24 * time.Hour

var errTokenUnusable = errors.New("token unusable")

type tokenPool struct {
	cache  cache.Cache
	client *http.Client
	log    *slog.Logger
	sf     singleflight.Group

	tokens []string

	mu      sync.Mutex
	current int
}

func newTokenPool(n int, client *http.Client, c cache.Cache, log *slog.Logger) *tokenPool {
	return &tokenPool{tokens: make([]string, n), client: client, cache: c, log: log}
}

func (p *tokenPool) cacheKey(idx int) string {
	return fmt.Sprintf("mxm:token:%d", idx)
}

func (p *tokenPool) ready() (string, int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for range p.tokens {
		idx := p.current
		p.current = (p.current + 1) % len(p.tokens)
		if p.tokens[idx] != "" {
			return p.tokens[idx], idx, true
		}
	}
	return "", -1, false
}

func (p *tokenPool) nextEmpty() (int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for range p.tokens {
		idx := p.current
		p.current = (p.current + 1) % len(p.tokens)
		if p.tokens[idx] == "" {
			return idx, true
		}
	}
	return -1, false
}

func (p *tokenPool) store(idx int, token string) {
	p.mu.Lock()
	p.tokens[idx] = token
	p.mu.Unlock()
}

func (p *tokenPool) get(ctx context.Context) (string, int, error) {
	if token, idx, ok := p.ready(); ok {
		return token, idx, nil
	}
	for range p.tokens {
		idx, ok := p.nextEmpty()
		if !ok {
			break
		}
		token, err := p.load(ctx, idx)
		if err != nil {
			if errors.Is(err, providers.ErrRateLimited) {
				p.log.Debug("token fetch rate limited, trying next slot", "slot", idx)
				continue
			}
			return "", -1, err
		}
		return token, idx, nil
	}
	if token, idx, ok := p.ready(); ok {
		return token, idx, nil
	}
	return "", -1, providers.ErrRateLimited
}

func (p *tokenPool) load(ctx context.Context, idx int) (string, error) {
	v, err, _ := p.sf.Do(p.cacheKey(idx), func() (any, error) {
		if p.cache != nil {
			val, status, err := cache.Get[string](ctx, p.cache, p.cacheKey(idx))
			if err == nil && status == cache.Hit && val != "" {
				p.store(idx, val)
				return val, nil
			}
		}
		token, err := p.fetch(ctx)
		if err != nil {
			return "", err
		}
		p.store(idx, token)
		if p.cache != nil {
			if err := cache.Set(ctx, p.cache, p.cacheKey(idx), token, tokenTTL); err != nil {
				p.log.Warn("token cache set failed", "slot", idx, "err", err)
			}
		}
		p.log.Debug("token ready", "slot", idx, "token", token[:8])
		return token, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (p *tokenPool) retire(idx int) {
	p.mu.Lock()
	had := p.tokens[idx] != ""
	p.tokens[idx] = ""
	p.mu.Unlock()
	if !had {
		return
	}
	if p.cache != nil {
		p.cache.Delete(context.Background(), p.cacheKey(idx))
	}
	go p.refreshSlot(idx)
}

func (p *tokenPool) refreshSlot(idx int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := p.load(ctx, idx); err != nil {
		p.log.Warn("token slot refresh failed", "slot", idx, "err", err)
	}
}

func (p *tokenPool) fetch(ctx context.Context) (string, error) {
	params := url.Values{"user_language": {"en"}, "app_id": {appID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"token.get?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("token read: %w", err)
	}

	var res struct {
		Message struct {
			Body struct {
				UserToken string `json:"user_token"`
			} `json:"body"`
			Header struct {
				Hint       string `json:"hint"`
				StatusCode int    `json:"status_code"`
			} `json:"header"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		p.log.Debug("token decode failed", "body", string(raw[:min(len(raw), 256)]))
		return "", fmt.Errorf("token decode: %w", err)
	}

	if res.Message.Header.StatusCode == 401 && res.Message.Header.Hint == "captcha" {
		return "", providers.ErrRateLimited
	}
	if res.Message.Header.StatusCode != 200 || res.Message.Body.UserToken == "" {
		return "", fmt.Errorf("token api %d (%s)", res.Message.Header.StatusCode, res.Message.Header.Hint)
	}
	return res.Message.Body.UserToken, nil
}
