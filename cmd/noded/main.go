// Command noded runs the Colony node engine without a window. It is the
// headless fallback for machines that cannot render the GUI, and the harness the
// Wails app starts internally. It detects hardware, connects to the Coordinator,
// and serves the local dashboard API on port 9090.
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/zonnhq/node-gui/internal/config"
	"github.com/zonnhq/node-gui/internal/daemon"
	"github.com/zonnhq/node-gui/internal/localapi"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("node: ")

	coordinator := flag.String("coordinator", "", "override the Coordinator address (host:port)")
	localAddr := flag.String("local-addr", localapi.DefaultAddr, "address for the local dashboard API (loopback only by default)")
	enrollCode := flag.String("enroll", "", "enroll this node with a code from Zonn Console, then keep running")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *coordinator != "" {
		cfg.CoordinatorURL = *coordinator
		if err := config.Save(cfg); err != nil {
			log.Printf("save config: %v", err)
		}
	}

	d := daemon.New(cfg)
	specs := d.Specs()
	log.Printf("node %q on %s (%d cores, %d GB RAM, GPU %q)",
		cfg.NodeName, specs.OS, specs.CPUCores, specs.RAMGB, specs.GPUModel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	api := localapi.New(d, *localAddr)
	go func() {
		log.Printf("local dashboard API on %s", *localAddr)
		if err := api.ListenAndServe(); err != nil {
			log.Printf("local api stopped: %v", err)
		}
	}()

	if *enrollCode != "" {
		if err := d.Enroll(ctx, *enrollCode); err != nil {
			log.Fatalf("enroll: %v", err)
		}
		log.Printf("enrolled as node %s", d.Config().NodeID)
	}

	go d.Run(ctx)

	<-ctx.Done()
	log.Print("shutting down")
	d.Shutdown()
}
