package core

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ChainSafe/log15"

	monitoring "hexbridge/utils/monitoring"
)

type Core struct {
	Registry []Chain
	route    *Router
	log      log15.Logger
	sysErr   <-chan error
}

func NewCore(sysErr <-chan error) *Core {
	return &Core{
		Registry: make([]Chain, 0),
		route:    NewRouter(log15.New("system", "router")),
		log:      log15.New("system", "core"),
		sysErr:   sysErr,
	}
}

// AddChain registers the chain in the Registry and calls Chain.SetRouter()
func (c *Core) AddChain(chain Chain) {
	c.Registry = append(c.Registry, chain)
	chain.SetRouter(c.route)
}

// Start will call all registered chains' Start methods and block forever (or until signal is received)
func (c *Core) Start() {
	for _, chain := range c.Registry {
		err := chain.Start()
		if err != nil {
			c.log.Error(
				"failed to start chain",
				"chain", chain.Id(),
				"err", err,
			)
			return
		}
		c.log.Info(fmt.Sprintf("Started %s chain", chain.Name()))
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigc)
	monitoring.Message("Hexbridge-relay server started successfully.")
	defer monitoring.Message("Hexbridge-relay server ended")

	// Block here and wait for a signal
	select {
	case err := <-c.sysErr:
		monitoring.Error(err)
		c.log.Error("FATAL ERROR. Shutting down.", "err", err)
	case <-sigc:
		errParam := "interrupt received, shutting down now"
		monitoring.Error(errors.New(errParam))
		c.log.Warn(errParam)
	}

	// Signal chains to shutdown
	for _, chain := range c.Registry {
		chain.Stop()
	}
}

func (c *Core) Errors() <-chan error {
	return c.sysErr
}
