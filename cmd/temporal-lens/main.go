package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	lens "github.com/RTradeLtd/Lens"
	"github.com/RTradeLtd/Lens/analyzer/images"
	"github.com/RTradeLtd/Lens/logs"
	"github.com/RTradeLtd/Lens/search"
	"github.com/RTradeLtd/Lens/server"
	"github.com/RTradeLtd/cmd"
	"github.com/RTradeLtd/config"
	"github.com/RTradeLtd/rtfs"
)

var (
	// Version denotes the tag of this build
	Version string

	// Edition indicates the this build's type
	Edition string

	// flag configuration
	cfgPath = flag.String("cfg", os.Getenv("CONFIG_DAG"),
		"path to Temporal configuration")
	modelPath = flag.String("models", "/tmp",
		"path to TensorFlow models")
	dsPath = flag.String("datastore", "/data/lens/badgerds-lens",
		"path to Badger datastore")
	logPath = flag.String("logpath", "",
		"path to write logs to - leave blank for stdout")
	devMode = flag.Bool("dev", false,
		"enable dev mode")
)

var commands = map[string]cmd.Cmd{
	"v2": cmd.Cmd{
		Blurb: "start the Lens V2 server",
		Action: func(cfg config.TemporalConfig, args map[string]string) {
			// set up logger
			l, err := logs.NewLogger(*logPath, *devMode)
			if err != nil {
				log.Fatal("failed to instantiate logger:", err.Error())
			}
			defer l.Sync()

			// instantiate ipfs connection
			var ipfsURL = fmt.Sprintf("%s:%s", cfg.IPFS.APIConnection.Host, cfg.IPFS.APIConnection.Port)
			l.Infow("instantiating IPFS connection", "ipfs.url", ipfsURL)
			manager, err := rtfs.NewManager(ipfsURL, "", 1*time.Minute)
			if err != nil {
				l.Fatalw("failed to instantiate ipfs manager", "error", err)
			}

			// instantiate tensorflow wrapper
			l.Infow("instantiating tensorflow wrappers", "tensorflow.models", *modelPath)
			tf, err := images.NewAnalyzer(images.ConfigOpts{
				ModelLocation: *modelPath,
			}, l.Named("analyzer").Named("images"))
			if err != nil {
				l.Fatalw("failed to instantiate image analyzer", "error", err)
			}

			// create lens v2 service
			l.Info("instantiating Lens V2")
			srv, err := lens.NewV2(lens.V2Options{}, manager, tf, l)
			if err != nil {
				l.Fatalw("failed to instantiate Lens V2", "error", err)
			}

			// set up interrupts
			var stop = make(chan bool)
			var signals = make(chan os.Signal)
			signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
			go func() {
				<-signals
				stop <- true
			}()

			// go!
			l.Infow("spinning up server", "config", cfg.Services.Lens)
			if err := server.RunV2(stop, l, srv, cfg.Services.Lens); err != nil {
				l.Fatalw("error encountered on server run", "error", err)
			}
		},
	},
	"v1": cmd.Cmd{
		Blurb:       "start Lens V1 server",
		Description: "Start the Lens meta data extraction service, which includes the API",
		Action: func(cfg config.TemporalConfig, args map[string]string) {
			l, err := logs.NewLogger(*logPath, *devMode)
			if err != nil {
				log.Fatal("failed to instantiate logger:", err.Error())
			}
			defer l.Sync()

			l = l.With(
				"version", Version,
				"edition", Edition)
			if *logPath != "" {
				println("logger initialized - output will be written to", *logPath)
			}

			// handle graceful shutdown
			ctx, cancel := context.WithCancel(context.Background())
			signals := make(chan os.Signal)
			signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
			go func() {
				<-signals
				cancel()
			}()

			// let's goooo
			if err := server.Run(
				ctx,
				cfg.Services.Lens.URL,
				server.Metadata{
					Version: Version,
					Edition: Edition,
				},
				lens.ConfigOpts{
					UseChainAlgorithm: true,
					DataStorePath:     *dsPath,
					ModelsPath:        *modelPath,
				},
				cfg,
				l.Named("server"),
			); err != nil {
				log.Fatal(err)
			}
		},
	},
	"migrate": cmd.Cmd{
		Blurb:       "Used to migrate teh datastore",
		Description: "Performs a complete migration of the old datastore to new datastore",
		Action: func(cfg config.TemporalConfig, args map[string]string) {
			im, err := rtfs.NewManager(
				fmt.Sprintf("%s:%s", cfg.IPFS.APIConnection.Host, cfg.IPFS.APIConnection.Port),
				"",
				5*time.Minute)
			if err != nil {
				log.Fatal(err)
			}
			s, err := search.NewService(*dsPath)
			if err != nil {
				log.Fatal(err)
			}
			defer s.Close()
			entriesToMigrate, err := s.GetEntries()
			if err != nil {
				log.Fatal(err)
			}
			if err = s.MigrateEntries(entriesToMigrate, im, true); err != nil {
				log.Fatal(err)
			}
		},
	},
}

func main() {
	if Version == "" {
		Version = "unknown"
	}

	// create app
	tlens := cmd.New(commands, cmd.Config{
		Name:     "Lens",
		ExecName: "temporal-lens",
		Version:  fmt.Sprintf("%s (%s edition)", Version, Edition),
		Desc:     "Lens is a tool to aid content discovery fro the distributed web",
	})

	// run no-config commands, exit if command was run
	flag.Parse()
	if exit := tlens.PreRun(flag.Args()); exit == cmd.CodeOK {
		os.Exit(0)
	}

	// load config
	if cfgPath == nil || *cfgPath == "" {
		log.Fatal("no configuration file provided - set CONFIG_DAG or use the --cfg flag")
	}
	tCfg, err := config.LoadConfig(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	// load arguments
	flags := map[string]string{
		"configDag":     *cfgPath,
		"certFilePath":  tCfg.API.Connection.Certificates.CertPath,
		"keyFilePath":   tCfg.API.Connection.Certificates.KeyPath,
		"listenAddress": tCfg.API.Connection.ListenAddress,

		"dbPass": tCfg.Database.Password,
		"dbURL":  tCfg.Database.URL,
		"dbUser": tCfg.Database.Username,
	}

	// execute
	os.Exit(tlens.Run(*tCfg, flags, flag.Args()))
}
