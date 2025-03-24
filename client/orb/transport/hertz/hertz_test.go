package hertz

import (
	"context"
	"os"
	"testing"

	"github.com/go-orb/go-orb/log"
	"github.com/go-orb/go-orb/registry"
	"github.com/go-orb/go-orb/server"
	"github.com/go-orb/go-orb/types"
	"github.com/go-orb/plugins/client/tests"
	"github.com/stretchr/testify/suite"

	"github.com/go-orb/plugins-experimental/server/hertz"

	echohandler "github.com/go-orb/plugins/client/tests/handler/echo"
	echoproto "github.com/go-orb/plugins/client/tests/proto/echo"

	// Blank imports here are fine.
	_ "github.com/go-orb/plugins/codecs/json"
	_ "github.com/go-orb/plugins/codecs/proto"
	_ "github.com/go-orb/plugins/codecs/yaml"
	_ "github.com/go-orb/plugins/log/slog"
	_ "github.com/go-orb/plugins/registry/mdns"
)

func setupServer(sn string) (*tests.SetupData, error) {
	ctx, cancel := context.WithCancel(context.Background())

	setupData := &tests.SetupData{}

	sv := "v1.0.0"

	logger, err := log.New()
	if err != nil {
		cancel()

		return nil, err
	}

	reg, err := registry.New(nil, &types.Components{}, logger)
	if err != nil {
		cancel()

		return nil, err
	}

	hInstance := new(echohandler.Handler)
	hRegister := echoproto.RegisterStreamsHandler(hInstance)

	ep1, err := hertz.New(
		sn, sv,
		hertz.NewConfig(
			server.WithEntrypointName("http"),
			hertz.WithHandlers(hRegister),
			hertz.WithInsecure(),
		),
		logger,
		reg,
	)
	if err != nil {
		cancel()

		return nil, err
	}

	ep2, err := hertz.New(
		sn, sv,
		hertz.NewConfig(
			server.WithEntrypointName("h2c"),
			hertz.WithHandlers(hRegister),
			hertz.WithInsecure(),
			hertz.WithAllowH2C(),
		),
		logger,
		reg,
	)
	if err != nil {
		cancel()

		return nil, err
	}

	setupData.Logger = logger
	setupData.Registry = reg
	setupData.Entrypoints = []server.Entrypoint{ep1, ep2}
	setupData.Ctx = ctx
	setupData.Stop = cancel

	return setupData, nil
}

func newSuite() *tests.TestSuite {
	s := tests.NewSuite(setupServer, []string{"hertzhttp", "hertzh2c"})
	// s.Debug = true
	return s
}

func TestSuite(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping testing in CI environment")
	}

	// Run the tests.
	suite.Run(t, newSuite())
}
