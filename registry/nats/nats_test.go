package nats

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/go-orb/plugins/codecs/json"
	_ "github.com/go-orb/plugins/log/slog"
	"github.com/go-orb/plugins/registry/tests"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	log "github.com/go-orb/go-orb/log"
	"github.com/go-orb/go-orb/registry"
	"github.com/go-orb/go-orb/types"
)

func createServer() (*tests.TestSuite, func() error, error) {
	logger, err := log.New(log.WithLevel("DEBUG"))
	if err != nil {
		log.Error("while creating a logger", err)
		return nil, func() error { return nil }, errors.New("while creating a logger")
	}

	var (
		started bool

		regOne   registry.Registry
		regTwo   registry.Registry
		regThree registry.Registry
	)

	logger.Info("starting NATS server")

	// start the NATS with JetStream server
	addr, cleanup, err := natsServer()
	if err != nil {
		log.Error("failed to setup NATS server", err)
	}

	// Sometimes the nats server has isssues with starting, so we attempt 5
	// times.
	for i := 0; i < 5; i++ {
		cfg, err := NewConfig(types.ServiceName("test.service"), nil, WithAddress(addr))
		if err != nil {
			log.Error("failed to create config", err)
		}

		regOne = New("", "", cfg, logger)
		err = regOne.Start(context.Background())
		if err != nil {
			time.Sleep(time.Second)
			continue
		}

		regTwo = New("", "", cfg, logger)
		regTwo.Start(context.Background()) //nolint:errcheck

		regThree = New("", "", cfg, logger)
		regThree.Start(context.Background()) //nolint:errcheck

		started = true
	}

	if !started {
		log.Error("failed to start NATS server", err)
		return nil, func() error { return nil }, errors.New("failed to start nats server")
	}

	s := tests.CreateSuite(logger, []registry.Registry{regOne, regTwo, regThree}, 0, 0)
	return s, cleanup, nil
}

func getFreeLocalhostAddress() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	return l.Addr().String(), l.Close()
}

func natsServer() (string, func() error, error) {
	addr, err := getFreeLocalhostAddress()
	if err != nil {
		return "", nil, err
	}
	host := strings.Split(addr, ":")[0]
	port, _ := strconv.Atoi(strings.Split(addr, ":")[1]) //nolint:errcheck

	natsCmd, err := filepath.Abs(filepath.Join("./test/bin/", runtime.GOOS+"_"+runtime.GOARCH, "nats-server"))
	if err != nil {
		return addr, nil, err
	}

	args := []string{"--addr", host, "--port", strconv.Itoa(port), "-js"}
	cmd := exec.Command(natsCmd, args...)
	if err := cmd.Start(); err != nil {
		return addr, nil, fmt.Errorf("failed starting command: %w", err)
	}

	cleanup := func() error {
		if cmd.Process == nil {
			return nil
		}

		if runtime.GOOS == "windows" {
			if err := cmd.Process.Kill(); err != nil {
				return fmt.Errorf("failed to kill the nats server: %w", err)
			}
		} else { // interrupt is not supported in windows
			if err := cmd.Process.Signal(os.Interrupt); err != nil {
				return fmt.Errorf("failed to kill the nats server: %w", err)
			}
		}

		return nil
	}

	return addr, cleanup, nil
}

func TestSuite(t *testing.T) {
	s, cleanup, err := createServer()
	require.NoError(t, err, "while creating a server")

	// Run the tests.
	suite.Run(t, s)

	require.NoError(t, cleanup(), "while cleaning up")
}

func BenchmarkGetService(b *testing.B) {
	b.StopTimer()

	s, cleanup, err := createServer()
	require.NoError(b, err, "while creating a server")

	s.BenchmarkGetService(b)

	require.NoError(b, cleanup(), "while cleaning up")
}

func BenchmarkGetServiceWithNoNodes(b *testing.B) {
	b.StopTimer()

	s, cleanup, err := createServer()
	require.NoError(b, err, "while creating a server")

	s.BenchmarkGetServiceWithNoNodes(b)

	require.NoError(b, cleanup(), "while cleaning up")
}
