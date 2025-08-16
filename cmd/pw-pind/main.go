package main

import (
	"context"
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/facebookincubator/go-belt"
	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/facebookincubator/go-belt/tool/logger/implementation/logrus"
	"github.com/xaionaro-go/observability"
	"github.com/xaionaro-go/pw-pin/pkg/pwpin"
	"github.com/xaionaro-go/xpath"
)

func main() {
	logLevel := logger.LevelInfo
	flag.Var(&logLevel, "log-level", "Set the log level (debug, info, warn, error, fatal)")
	netPprofAddr := flag.String(
		"go-net-pprof-addr",
		"",
		"address to listen to for net/pprof requests",
	)
	configPathRaw := flag.String(
		"config-file",
		"~/.config/pw-pin/config.yaml",
		"path to the configuration file",
	)
	generateSampleConfig := flag.Bool(
		"generate-sample-config",
		false,
		"generate a sample configuration file and exit",
	)
	flag.Parse()

	if *generateSampleConfig {
		printSampleConfig()
		os.Exit(0)
	}

	configPath := must(xpath.Expand(*configPathRaw))
	configBytes := must(os.ReadFile(configPath))

	var config pwpin.Config
	assertNoError(config.Parse(configBytes))

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

	sp := pwpin.New()
	sp.SetConfig(config)

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

	assertNoError(sp.ServeContext(ctx))
}
