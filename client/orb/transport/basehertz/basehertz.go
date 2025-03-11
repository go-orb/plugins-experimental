// Package basehertz contains a base transport which is used by hertz transports.
package basehertz

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"

	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/go-orb/go-orb/client"
	"github.com/go-orb/go-orb/codecs"
	"github.com/go-orb/go-orb/log"
	"github.com/go-orb/go-orb/util/metadata"
	"github.com/go-orb/go-orb/util/orberrors"
	"github.com/go-orb/plugins/client/orb"
)

//nolint:gochecknoglobals
var stdHeaders = []string{"Content-Length", "Content-Type", "Date", "Server"}

var _ (orb.Transport) = (*Transport)(nil)

// TransportClientCreator is a factory for a client transport.
type TransportClientCreator func() (*hclient.Client, error)

// Transport is a go-orb/plugins/client/orb compatible transport.
type Transport struct {
	name          string
	logger        log.Logger
	clientCreator TransportClientCreator
	hclient       *hclient.Client
	scheme        string
}

// Start starts the transport.
func (t *Transport) Start() error {
	return nil
}

// Stop stop the transport.
func (t *Transport) Stop(_ context.Context) error {
	if t.hclient != nil {
		t.hclient.CloseIdleConnections()
	}

	return nil
}

// Name returns the name of this transport.
func (t *Transport) Name() string {
	return t.name
}

// Request does the actual rpc request to the server.
func (t *Transport) Request(
	ctx context.Context,
	req *client.Req[any, any],
	result any,
	opts *client.CallOptions,
) error {
	codec, err := codecs.GetEncoder(opts.ContentType, req.Req())
	if err != nil {
		return orberrors.ErrBadRequest.Wrap(err)
	}

	// Encode the request into a *bytes.Buffer{}.
	buff := bytes.NewBuffer(nil)
	if err := codec.NewEncoder(buff).Encode(req.Req()); err != nil {
		return orberrors.ErrBadRequest.Wrap(err)
	}

	node, err := req.Node(ctx, opts)
	if err != nil {
		return orberrors.From(err)
	}

	// Create a hertz request.
	hReq := &protocol.Request{}
	hReq.SetMethod(consts.MethodPost)
	hReq.SetBodyStream(buff, buff.Len())
	hReq.Header.SetContentTypeBytes([]byte(opts.ContentType))
	hReq.Header.Set("Accept", opts.ContentType)
	hReq.SetRequestURI(fmt.Sprintf("%s://%s%s", t.scheme, node.Address, req.Endpoint()))

	// Set metadata key=value to request headers.
	md, ok := metadata.Outgoing(ctx)
	if ok {
		for name, value := range md {
			hReq.Header.Set(name, value)
		}
	}

	// Get the client
	if t.hclient == nil {
		hclient, err := t.clientCreator()
		if err != nil {
			return err
		}

		t.hclient = hclient
	}

	// Run the request.
	hRes := &protocol.Response{}

	err = t.hclient.DoTimeout(ctx, hReq, hRes, opts.RequestTimeout)
	if err != nil {
		return orberrors.From(err)
	}

	// Read into a bytes.Buffer.
	buff = bytes.NewBuffer(hRes.Body())

	if opts.ResponseMetadata != nil {
		for _, v := range hRes.Header.GetHeaders() {
			k := string(v.GetKey())

			// Skip std headers.
			if slices.Contains(stdHeaders, k) {
				continue
			}

			opts.ResponseMetadata[strings.ToLower(k)] = string(v.GetValue())
		}
	}

	if hRes.StatusCode() != consts.StatusOK {
		return orberrors.HTTP(hRes.StatusCode())
	}

	// Decode the response into `result`.
	err = codec.NewDecoder(buff).Decode(result)
	if err != nil {
		return orberrors.ErrBadRequest.Wrap(err)
	}

	return nil
}

// NewTransport creates a Transport with a custom http.Client.
func NewTransport(name string, logger log.Logger, scheme string, clientCreator TransportClientCreator,
) (orb.TransportType, error) {
	return orb.TransportType{Transport: &Transport{
		name:          name,
		logger:        logger,
		scheme:        scheme,
		clientCreator: clientCreator,
	}}, nil
}
