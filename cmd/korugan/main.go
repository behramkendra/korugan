// Command korugan runs the whole platform: migrations, connector sync
// loop, analyzers and the HTTP API — one binary, PostgreSQL as the only
// dependency.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/behramkendra/korugan/internal/action"
	"github.com/behramkendra/korugan/internal/ai"
	aiprovider "github.com/behramkendra/korugan/internal/ai/provider"
	"github.com/behramkendra/korugan/internal/analysis"
	"github.com/behramkendra/korugan/internal/config"
	"github.com/behramkendra/korugan/internal/connector"
	_ "github.com/behramkendra/korugan/internal/connector/cloudflare" // register adapter
	"github.com/behramkendra/korugan/internal/crypto"
	"github.com/behramkendra/korugan/internal/domain"
	"github.com/behramkendra/korugan/internal/httpapi"
	"github.com/behramkendra/korugan/internal/ingest"
	"github.com/behramkendra/korugan/internal/obs"
	"github.com/behramkendra/korugan/internal/settings"
	"github.com/behramkendra/korugan/internal/store"
)

func main() {
	cfg, err := config.Load()
	log := obs.NewLogger(cfg.LogLevel)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("store open", "err", err)
		os.Exit(1)
	}
	defer st.Close()
	log.Info("store ready, migrations applied")

	// Sealed settings: enabled when a master key is present. Stored
	// credentials take precedence over environment variables.
	var sealer *crypto.Sealer
	if mk := cfg.MasterKeyB64; mk != "" {
		sealer, err = crypto.NewSealer(mk)
		if err != nil {
			log.Error("master key invalid", "err", err)
			os.Exit(1)
		}
		log.Info("sealed credential storage enabled")
	} else {
		log.Warn("KORUGAN_MASTER_KEY not set — sealed settings disabled; credentials come from environment only")
	}
	settingsSvc := settings.New(st, sealer)

	// Connectors: prefer a sealed Cloudflare token, fall back to the env var.
	cfToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	if settingsSvc.Enabled() {
		if stored, err := settingsSvc.CloudflareToken(ctx); err != nil {
			log.Warn("could not read stored cloudflare token", "err", err)
		} else if stored != "" {
			cfToken = stored
			log.Info("using stored Cloudflare token from sealed settings")
		}
	}
	var conns []connector.Connector
	if tok := cfToken; tok != "" {
		c, err := connector.New(domain.ProviderCloudflare, map[string]string{"api_token": tok})
		if err != nil {
			log.Error("cloudflare connector", "err", err)
			os.Exit(1)
		}
		if err := c.Validate(ctx); err != nil {
			log.Error("cloudflare token invalid", "err", err)
			os.Exit(1)
		}
		conns = append(conns, c)
		log.Info("cloudflare connector validated")
	} else {
		log.Warn("CLOUDFLARE_API_TOKEN not set — running without connectors")
	}

	engine := buildEngine(ctx, st, settingsSvc, log)
	if engine.Enabled() {
		log.Info("ai engine enabled")
	} else {
		log.Info("zero-key mode: AI features off until an LLM key is configured")
	}

	// Action service: resolve the writable connector per provider from the
	// connectors constructed above.
	actionsvc := &action.Service{
		Store: st, Log: log,
		Resolve: func(p domain.Provider) (connector.WriteConnector, bool) {
			for _, c := range conns {
				if c.Info().Provider == p {
					return connector.AsWriter(c)
				}
			}
			return nil, false
		},
	}

	if len(conns) > 0 {
		poller := &ingest.Poller{
			Store: st, Log: log, Interval: cfg.PollInterval,
			Analysis:   &analysis.Runner{Store: st, Log: log},
			Connectors: conns,
		}
		go poller.Run(ctx)
	}

	api := &httpapi.Server{Store: st, Engine: engine, Actions: actionsvc, Settings: settingsSvc, Log: log, APIToken: os.Getenv("KORUGAN_API_TOKEN")}
	srv := &http.Server{Addr: cfg.Addr, Handler: api.Router(), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()

	log.Info("korugan listening", "addr", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http server", "err", err)
		os.Exit(1)
	}
	log.Info("shutdown complete")
}

// buildEngine maps KORUGAN_LLM_* env vars onto tier assignments.
// KORUGAN_LLM_PROVIDER: openrouter|openai|deepseek|ollama|anthropic
// KORUGAN_LLM_MODEL:    model id used for every tier (v0.1 single-model)
// KORUGAN_LLM_API_KEY:  the user's own key (BYOK)
// KORUGAN_LLM_BASE_URL: optional override (self-hosted gateways)
func buildEngine(ctx context.Context, st *store.Store, settingsSvc *settings.Service, log interface{ Warn(string, ...any) }) *ai.Engine {
	name := os.Getenv("KORUGAN_LLM_PROVIDER")
	model := os.Getenv("KORUGAN_LLM_MODEL")
	key := os.Getenv("KORUGAN_LLM_API_KEY")
	base := os.Getenv("KORUGAN_LLM_BASE_URL")

	// Stored (sealed) LLM config takes precedence over environment.
	if settingsSvc.Enabled() {
		if cfg, err := settingsSvc.LLM(ctx); err == nil && cfg != nil {
			name, model, key, base = cfg.Provider, cfg.Model, cfg.APIKey, cfg.BaseURL
		}
	}
	if name == "" {
		return ai.NewEngine(nil, st)
	}

	kind := aiprovider.KindOpenAICompatible
	switch name {
	case "anthropic":
		kind = aiprovider.KindAnthropic
	case "openrouter":
		if base == "" {
			base = "https://openrouter.ai/api/v1"
		}
	case "deepseek":
		if base == "" {
			base = "https://api.deepseek.com/v1"
		}
	case "ollama":
		if base == "" {
			base = "http://127.0.0.1:11434/v1"
		}
	case "openai":
		// default base
	default:
		log.Warn("unknown KORUGAN_LLM_PROVIDER; expected openrouter|openai|deepseek|ollama|anthropic", "got", name)
	}
	if model == "" || (key == "" && name != "ollama") {
		log.Warn("incomplete LLM config: KORUGAN_LLM_MODEL and KORUGAN_LLM_API_KEY required (key optional for ollama)")
		return ai.NewEngine(nil, st)
	}
	p, err := aiprovider.New(aiprovider.Config{Kind: kind, Name: name, BaseURL: base, APIKey: key})
	if err != nil {
		log.Warn("llm provider init failed", "err", err)
		return ai.NewEngine(nil, st)
	}
	a := ai.Assignment{
		Provider:      p,
		Model:         model,
		PriceInPer1K:  envFloat("KORUGAN_LLM_PRICE_IN_PER_1K"),
		PriceOutPer1K: envFloat("KORUGAN_LLM_PRICE_OUT_PER_1K"),
	}
	engine := ai.NewEngine(map[ai.Tier]ai.Assignment{
		ai.TierFast: a, ai.TierBalanced: a, ai.TierDeep: a,
	}, st)
	if daily, monthly := envFloat("KORUGAN_LLM_BUDGET_DAILY_USD"), envFloat("KORUGAN_LLM_BUDGET_MONTHLY_USD"); daily > 0 || monthly > 0 {
		engine.SetBudget(ai.Budget{DailyUSD: daily, MonthlyUSD: monthly}, st)
	}
	return engine
}

func envFloat(key string) float64 {
	v := os.Getenv(key)
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}
