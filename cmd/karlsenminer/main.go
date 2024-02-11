package main

import (
	"fmt"
	"os"

	"github.com/karlsen-network/karlsend/cmd/karlsenminer/custoption"
	"github.com/karlsen-network/karlsend/util"

	"github.com/karlsen-network/karlsend/version"

	"github.com/pkg/errors"

	_ "net/http/pprof"

	"github.com/karlsen-network/karlsend/domain/consensus/utils/pow"
	"github.com/karlsen-network/karlsend/infrastructure/os/signal"
	"github.com/karlsen-network/karlsend/util/panics"
	"github.com/karlsen-network/karlsend/util/profiling"
)

func main() {
	defer panics.HandlePanic(log, "MAIN", nil)
	interrupt := signal.InterruptListener()

	cfg, err := parseConfig()
	if err != nil {
		printErrorAndExit(errors.Errorf("Error parsing command-line arguments: %s", err))
	}
	defer backendLog.Close()

	// Show version at startup.
	log.Infof("Version %s", version.Version())
	log.Infof("Using KarlsenHashV2 impl: %s", pow.GetHashingAlgoVersion())
	if !cfg.Testnet && !cfg.Devnet && !cfg.Simnet {
		log.Warnf("You are trying to connect to Mainnet")
		log.Errorf("This version is using KarlsenHashV2, please add --testnet parameter")
		os.Exit(42)
	}

	// Enable http profiling server if requested.
	if cfg.Profile != "" {
		profiling.Start(cfg.Profile, log)
	}

	client, err := newMinerClient(cfg)
	if err != nil {
		panic(errors.Wrap(err, "error connecting to the RPC server"))
	}
	defer client.Disconnect()

	miningAddr, err := util.DecodeAddress(cfg.MiningAddr, cfg.ActiveNetParams.Prefix)
	if err != nil {
		printErrorAndExit(errors.Errorf("Error decoding mining address: %s", err))
	}

	customOpt := &custoption.Option{
		NumThreads:          7,
		Path:                cfg.HashesPath,
		DisableLocalDagFile: cfg.DisableLocalDagFile,
	}

	if customOpt.Path != "" {
		ok := custoption.CheckPath(customOpt.Path)
		if ok != nil {
			printErrorAndExit(errors.Errorf("Error wrong hashespath: %s", ok))
		}
	}

	doneChan := make(chan struct{})
	spawn("mineLoop", func() {
		err = mineLoop(client, cfg.NumberOfBlocks, *cfg.TargetBlocksPerSecond, cfg.MineWhenNotSynced, miningAddr, customOpt)
		if err != nil {
			panic(errors.Wrap(err, "error in mine loop"))
		}
		doneChan <- struct{}{}
	})

	select {
	case <-doneChan:
	case <-interrupt:
	}
}

func printErrorAndExit(err error) {
	fmt.Fprintf(os.Stderr, "%+v\n", err)
	os.Exit(1)
}
