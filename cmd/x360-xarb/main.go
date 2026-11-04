// Command x360-xarb serves Xbox Live arbitration, Live service 16 (see internal/xarb).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openpak/x360-common/erase"
	"github.com/openpak/x360-common/httplog"
	"github.com/openpak/x360-common/wire"
	"github.com/openpak/x360-common/xuacsapi"
	"github.com/openpak/x360-xarb/internal/xarb"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	addr := os.Getenv("X360_XARB_ADDR")
	if addr == "" {
		addr = ":10016"
	}
	key := os.Getenv("X360_GATEWAY_KEY")
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Second)
	store, err := xarb.OpenStore(dbCtx, os.Getenv("X360_XARB_DATABASE_URL"), xarb.Schema)
	dbCancel()
	if err != nil {
		log.Error("state: X360_XARB_DATABASE_URL is required and must be reachable", "err", err)
		os.Exit(2)
	}
	defer store.Close()
	h := &xarb.Handler{Store: store, Log: log}
	if u := os.Getenv("X360_XARB_STATPOST_URL"); u != "" { // e.g. http://openpak-x360-xstats:10007/xstats/statpost.ashx
		h.StatPost = xarb.PostTo(u, key, os.Getenv("X360_XARB_ARB_KEY"), &http.Client{Timeout: 5 * time.Second})
	}
	srv := &http.Server{Addr: addr, Handler: httplog.Wrap(wire.Guard(key, h)), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Account erasure (card #120): sessions hold XUIDs, which xuacs still knows
	// while the website finaliser runs (x360-xuacs is erased last).
	if u, k := os.Getenv("X360_XUACS_INTERNAL_URL"), os.Getenv("X360_XUACS_INTERNAL_KEY"); u != "" && k != "" {
		ids := &xuacsapi.Client{URL: u, Key: k}
		erase.Serve(ctx, "x360-xarb", func(c context.Context, account string) (int, error) {
			xs, err := ids.AccountXUIDs(c, account)
			if err != nil {
				return 0, err
			}
			return store.Erase(xs)
		})
	} else if os.Getenv("ACCOUNT_ERASE_KEY") != "" {
		log.Warn("account erase endpoint disabled: X360_XUACS_INTERNAL_URL and X360_XUACS_INTERNAL_KEY are not set")
	}
	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sc)
	}()
	log.Info("x360-xarb listening", "addr", addr, "statpost", h.StatPost != nil)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
