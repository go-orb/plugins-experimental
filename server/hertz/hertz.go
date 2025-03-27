// Package hertz contains a hertz server for go-orb.
package hertz

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/cloudwego/hertz/pkg/app/server"
	hconfig "github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/common/hlog"

	"github.com/go-orb/go-orb/config"
	"github.com/go-orb/go-orb/log"
	"github.com/go-orb/go-orb/registry"
	orbserver "github.com/go-orb/go-orb/server"
	"github.com/go-orb/go-orb/util/addr"
	"github.com/go-orb/plugins-experimental/server/hertz/internal/orblog"

	"github.com/hertz-contrib/http2/factory"
)

var _ orbserver.Entrypoint = (*Server)(nil)

// Server is the hertz Server for go-orb.
type Server struct {
	serviceName    string
	serviceVersion string
	epName         string

	config   *Config
	logger   log.Logger
	registry registry.Type

	address string
	hServer *server.Hertz

	started bool
}

// Start will create the listeners and start the server on the entrypoint.
func (s *Server) Start(ctx context.Context) error {
	if s.started {
		return nil
	}

	s.logger.Info("Starting", "address", s.config.Address)

	// Listen and close on that address, to see which port we get.
	l, err := net.Listen("tcp", s.config.Address)
	if err != nil {
		return err
	}

	s.address = l.Addr().String()

	if err := l.Close(); err != nil {
		return err
	}

	s.logger.Info("Got address", "address", s.address)

	hlog.SetLogger(orblog.NewLogger(s.logger))

	hopts := []hconfig.Option{server.WithHostPorts(s.address)}
	if s.config.H2C {
		hopts = append(hopts, server.WithH2C(true))
	}

	s.hServer = server.Default(hopts...)

	// Register handlers.
	for _, h := range s.config.OptHandlers {
		h(s)
	}

	if s.config.H2C || s.config.HTTP2 {
		// register http2 server factory
		s.hServer.AddProtocol("h2", factory.NewServerFactory())
	}

	errCh := make(chan error)
	go func(h *server.Hertz, errCh chan error) {
		errCh <- h.Run()
	}(s.hServer, errCh)

	if err := s.registryRegister(ctx); err != nil {
		return fmt.Errorf("failed to register the hertz server: %w", err)
	}

	s.started = true

	return nil
}

// Stop will stop the Hertz server(s).
func (s *Server) Stop(ctx context.Context) error {
	if !s.started {
		return nil
	}

	errChan := make(chan error)
	defer close(errChan)

	s.logger.Debug("Stopping")

	if err := s.registryDeregister(ctx); err != nil {
		return err
	}

	stopCtx, cancel := context.WithTimeoutCause(ctx, s.config.StopTimeout, errors.New("timeout while stopping the hertz server"))
	defer cancel()

	s.started = false

	return s.hServer.Shutdown(stopCtx)
}

// AddHandler adds a handler for later registration.
func (s *Server) AddHandler(handler orbserver.RegistrationFunc) {
	s.config.OptHandlers = append(s.config.OptHandlers, handler)
}

// Register executes a registration function on the entrypoint.
func (s *Server) Register(register orbserver.RegistrationFunc) {
	register(s)
}

// Network returns the network the entrypoint is listening on.
func (s *Server) Network() string {
	return s.config.Network
}

// Address returns the address the entrypoint is listening on.
func (s *Server) Address() string {
	return s.address
}

// Transport returns the client transport to use.
func (s *Server) Transport() string {
	if s.config.H2C {
		return "hertzh2c"
	} else if !s.config.Insecure {
		return "hertzhttps"
	}

	return "hertzhttp"
}

// String returns the entrypoint type; http.
func (s *Server) String() string {
	return Plugin
}

// Enabled returns if this entrypoint has been enbaled in config.
func (s *Server) Enabled() bool {
	return s.config.Enabled
}

// Name returns the entrypoint name.
func (s *Server) Name() string {
	return s.epName
}

// Type returns the component type.
func (s *Server) Type() string {
	return orbserver.EntrypointType
}

// Router returns the hertz server.
func (s *Server) Router() *server.Hertz {
	return s.hServer
}

func (s *Server) registryService() registry.ServiceNode {
	return registry.ServiceNode{
		Name:     s.serviceName,
		Version:  s.serviceVersion,
		Node:     s.Name(),
		Network:  s.Network(),
		Address:  s.Address(),
		Scheme:   s.Transport(),
		Metadata: make(map[string]string),
	}
}

func (s *Server) registryRegister(ctx context.Context) error {
	return s.registry.Register(ctx, s.registryService())
}

func (s *Server) registryDeregister(ctx context.Context) error {
	return s.registry.Deregister(ctx, s.registryService())
}

// Provide creates a new entrypoint for a single address. You can create
// multiple entrypoints for multiple addresses and ports. One entrypoint
// can serve a HTTP1 and HTTP2/H2C server.
func Provide(
	serviceName string,
	serviceVersion string,
	epName string,
	configs map[string]any,
	logger log.Logger,
	reg registry.Type,
	opts ...orbserver.Option,
) (orbserver.Entrypoint, error) {
	cfg := NewConfig(opts...)

	if err := config.Parse(nil, "", configs, cfg); err != nil && !errors.Is(err, config.ErrNoSuchKey) {
		return nil, err
	}

	return New(serviceName, serviceVersion, epName, cfg, logger, reg)
}

// New creates a hertz server by options.
func New(
	serviceName string,
	serviceVersion string,
	epName string,
	acfg any,
	logger log.Logger,
	reg registry.Type,
) (orbserver.Entrypoint, error) {
	cfg, ok := acfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("hertz invalid config: %v", cfg)
	}

	var err error

	cfg.Address, err = addr.GetAddress(cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("hertz validate addr '%s': %w", cfg.Address, err)
	}

	if err := addr.ValidateAddress(cfg.Address); err != nil {
		return nil, err
	}

	entrypoint := Server{
		serviceName:    serviceName,
		serviceVersion: serviceVersion,
		epName:         epName,

		config:   cfg,
		logger:   logger,
		registry: reg,
	}

	return &entrypoint, nil
}
