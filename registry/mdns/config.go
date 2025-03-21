package mdns

import (
	"github.com/go-orb/go-orb/registry"
)

// metaTransportKey is the key to use to store the scheme in metadata.
const metaTransportKey = "__mdns__transport"

// Name provides the name of this registry.
const Name = "mdns"

// Defaults.
//
//nolint:gochecknoglobals
var (
	DefaultDomain = "micro"
)

func init() {
	registry.Plugins.Add(Name, Provide)
}

// Config provides configuration for the mDNS registry.
type Config struct {
	registry.Config `yaml:",inline"`

	Domain string `json:"domain,omitempty" yaml:"domain,omitempty"`
}

// NewConfig creates a new config object.
func NewConfig(
	opts ...registry.Option,
) Config {
	cfg := Config{
		Config: registry.NewConfig(),
	}

	cfg.Config.Timeout = 500

	// Apply options.
	for _, o := range opts {
		o(&cfg)
	}

	return cfg
}

// WithDomain sets the mDNS domain.
func WithDomain(domain string) registry.Option {
	return func(c registry.ConfigType) {
		cfg, ok := c.(*Config)
		if ok {
			cfg.Domain = domain
		}
	}
}
