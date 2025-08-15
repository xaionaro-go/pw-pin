package main

import (
	"context"
	"flag"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/facebookincubator/go-belt"
	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/facebookincubator/go-belt/tool/logger/implementation/logrus"
	"github.com/xaionaro-go/observability"
	"github.com/xaionaro-go/simpleplumber/pkg/simpleplumber"
)

func main() {
	logLevel := logger.LevelInfo
	flag.Var(&logLevel, "log-level", "Set the log level (debug, info, warn, error, fatal)")
	netPprofAddr := flag.String(
		"go-net-pprof-addr",
		"",
		"address to listen to for net/pprof requests",
	)
	flag.Parse()

	l := logrus.Default().WithLevel(logLevel)
	ctx := context.Background()
	ctx = logger.CtxWithLogger(ctx, l)
	logger.Default = func() logger.Logger {
		return l
	}
	defer belt.Flush(ctx)

	if *netPprofAddr != "" {
		observability.Go(ctx, func(ctx context.Context) {
			logger.Infof(ctx, "starting to listen for net/pprof requests at '%s'", *netPprofAddr)
			logger.Error(ctx, http.ListenAndServe(*netPprofAddr, nil))
		})
	}

	appSelector := simpleplumber.Constraints{
		{Parameter: "node.name", Values: []string{"mpv"}, Op: simpleplumber.ConstraintOpEqual},
		{Parameter: "media.name", Values: []string{"1.webm - mpv"}, Op: simpleplumber.ConstraintOpEqual},
		{Parameter: "media.class", Values: []string{"Stream/Output/Audio"}, Op: simpleplumber.ConstraintOpEqual},
	}

	sp := simpleplumber.New()
	sp.SetConfig(&simpleplumber.Config{
		Routes: []simpleplumber.Route{ // the higher in the list, the higher priority
			{ // link app to specific output
				ShouldBeLinked:     true,
				InputNodesSelector: appSelector,
				OutputNodesSelector: simpleplumber.Constraints{
					{
						Parameter: "node.name",
						Values:    []string{"alsa_output.usb-R__DE_RODECaster_Duo_IR0037235-00.pro-output-0"},
						Op:        simpleplumber.ConstraintOpEqual,
					},
				},
			},
			{ // de-link app from all outputs
				ShouldBeLinked:      false,
				InputNodesSelector:  appSelector,
				OutputNodesSelector: simpleplumber.Constraints{},
			},
		},
	})

	logger.Infof(ctx, "started")

	observability.Go(ctx, func(ctx context.Context) {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sp.ActiveDataLocker.Do(ctx, func() {
					logger.Debugf(
						ctx,
						"active sinks: %d, active sources: %d, active ports: %d, active links: %d",
						len(sp.ActiveSinks), len(sp.ActiveSources), len(sp.ActivePorts), len(sp.ActiveLinks),
					)
				})
			}
		}
	})

	must(sp.ServeContext(ctx))
}
