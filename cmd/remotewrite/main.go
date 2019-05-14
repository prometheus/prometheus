// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage/remote"
	"golang.org/x/net/netutil"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	cfg := struct {
		configFile string

		localStoragePath    string
		remoteFlushDeadline model.Duration

		listenAddress  string
		httpTimeout    model.Duration
		maxConnections int

		promlogConfig promlog.Config
	}{
		promlogConfig: promlog.Config{},
	}
	a := kingpin.New(filepath.Base(os.Args[0]), "The Prometheus monitoring server")

	a.Version(version.Print("remotewrite"))

	a.HelpFlag.Short('h')

	a.Flag("config.file", "Prometheus configuration file path.").
		Default("prometheus.yml").StringVar(&cfg.configFile)

	a.Flag("web.listen-address", "Address to listen on for UI, API, and telemetry.").
		Default("0.0.0.0:9095").StringVar(&cfg.listenAddress)

	a.Flag("web.read-timeout",
		"Maximum duration before timing out read of the request, and closing idle connections.").
		Default("5m").SetValue(&cfg.httpTimeout)

	a.Flag("web.max-connections", "Maximum number of simultaneous connections.").
		Default("512").IntVar(&cfg.maxConnections)

	a.Flag("storage.tsdb.path", "Base path for metrics storage.").
		Default("data/").StringVar(&cfg.localStoragePath)

	a.Flag("storage.remote.flush-deadline", "How long to wait flushing sample on shutdown or config reload.").
		Default("1m").PlaceHolder("<duration>").SetValue(&cfg.remoteFlushDeadline)

	promlogflag.AddFlags(a, &cfg.promlogConfig)

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "Error parsing commandline arguments"))
		a.Usage(os.Args[1:])
		os.Exit(2)
	}

	logger := promlog.New(&cfg.promlogConfig)

	remoteWriteStorage := remote.NewWriteStorage(logger, cfg.localStoragePath, time.Duration(cfg.remoteFlushDeadline), true)

	// sync.Once is used to make sure we can close the channel at different execution stages(SIGTERM or when the config is loaded).
	type closeOnce struct {
		C     chan struct{}
		once  sync.Once
		Close func()
	}
	// Wait until the server is ready to handle reloading.
	reloadReady := &closeOnce{
		C: make(chan struct{}),
	}
	reloadReady.Close = func() {
		reloadReady.once.Do(func() {
			close(reloadReady.C)
		})
	}

	var g run.Group
	{
		// Termination handler.
		term := make(chan os.Signal, 1)
		signal.Notify(term, os.Interrupt, syscall.SIGTERM)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				// Don't forget to release the reloadReady channel so that waiting blocks can exit normally.
				select {
				case <-term:
					level.Warn(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
					reloadReady.Close()
				case <-cancel:
					reloadReady.Close()
					break
				}
				return nil
			},
			func(err error) {
				close(cancel)
			},
		)
	}
	{
		// Reload handler.
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				<-reloadReady.C

				for {
					select {
					case <-hup:
						if err := reloadConfig(cfg.configFile, logger, remoteWriteStorage.ApplyConfig); err != nil {
							level.Error(logger).Log("msg", "Error reloading config", "err", err)
						}
					case <-cancel:
						return nil
					}
				}

			},
			func(err error) {
				// Wait for any in-progress reloads to complete to avoid
				// reloading things after they have been shutdown.
				cancel <- struct{}{}
			},
		)
	}
	{
		// Initial configuration loading.
		cancel := make(chan struct{})
		g.Add(
			func() error {
				if err := reloadConfig(cfg.configFile, logger, remoteWriteStorage.ApplyConfig); err != nil {
					return errors.Wrapf(err, "error loading config from %q", cfg.configFile)
				}

				reloadReady.Close()

				<-cancel
				return nil
			},
			func(err error) {
				close(cancel)
				if err := remoteWriteStorage.Close(); err != nil {
					level.Error(logger).Log("msg", "Error remote write storage", "err", err)
				}
			},
		)
	}
	{
		// Web handler.
		cancel := make(chan struct{})
		g.Add(
			func() error {
				router := route.New()
				router.Get("/metrics", promhttp.Handler().ServeHTTP)
				listener, err := net.Listen("tcp", cfg.listenAddress)
				if err != nil {
					return err
				}
				listener = netutil.LimitListener(listener, cfg.maxConnections)

				mux := http.NewServeMux()
				mux.Handle("/", router)
				httpSrv := &http.Server{
					Handler: mux,
				}

				errCh := make(chan error)
				go func() {
					errCh <- httpSrv.Serve(listener)
				}()
				select {
				case e := <-errCh:
					return e
				case <-cancel:
					httpSrv.Shutdown(context.Background())
					return nil
				}
			},
			func(err error) {
				close(cancel)
			},
		)
	}
	if err := g.Run(); err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
	level.Info(logger).Log("msg", "See you next time!")
}

func reloadConfig(filename string, logger log.Logger, reloader func(*config.Config) error) (err error) {
	level.Info(logger).Log("msg", "Loading configuration file", "filename", filename)

	conf, err := config.LoadFile(filename)
	if err != nil {
		return errors.Wrapf(err, "couldn't load configuration (--config.file=%q)", filename)
	}

	if err := reloader(conf); err != nil {
		level.Error(logger).Log("msg", "Failed to apply configuration", "err", err)
	}

	level.Info(logger).Log("msg", "Completed loading of configuration file", "filename", filename)
	return nil
}
