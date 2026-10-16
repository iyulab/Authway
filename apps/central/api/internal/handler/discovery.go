package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/delegation"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// discoveryTTL is how long the authorization server's own document is kept
// before it is read again.
const discoveryTTL = time.Minute

// Discovery serves the provider's metadata as Authway offers it (OpenID
// Connect Discovery 1.0, RFC 8414): the authorization server's document,
// with what Authway adds and what the deployment does not offer taken out.
//
// The authorization server's document cannot say these itself — its grant,
// response type and logout lists are fixed in its code, and it has no place
// for the introspection endpoint.
// TODO(upstream: #1313) the introspection endpoint, once the authorization
// server can advertise it.
//
// The issuer stays the authorization server's: a client finds this document
// at the issuer's address only where the deployment routes that address here
// (docs/DEPLOYMENT.md, "Discovery document"); read at the API's own address it
// fails a client's issuer check, as it should.
type Discovery struct {
	engine    string
	apiURL    string
	exchange  bool
	approvals bool
	http      *http.Client
	logger    *zap.Logger

	mu      sync.Mutex
	cached  map[string]any
	fetched time.Time
}

// NewDiscovery returns a Discovery. engineURL is the authorization server's
// public address as this API reaches it, apiURL this API's public address;
// exchange and approvals say whether token exchange and approval requests are
// offered.
func NewDiscovery(engineURL, apiURL string, exchange, approvals bool, logger *zap.Logger) *Discovery {
	return &Discovery{
		engine:    strings.TrimRight(engineURL, "/") + "/.well-known/openid-configuration",
		apiURL:    strings.TrimRight(apiURL, "/"),
		exchange:  exchange,
		approvals: approvals,
		http:      &http.Client{Timeout: 10 * time.Second},
		logger:    logger,
	}
}

// Serve handles GET /.well-known/openid-configuration and
// /.well-known/oauth-authorization-server.
func (d *Discovery) Serve(c *fiber.Ctx) error {
	doc, err := d.document(c.UserContext())
	if err != nil {
		d.logger.Error("Reading the authorization server's discovery document failed", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusServiceUnavailable, "temporarily_unavailable", "The authorization server's discovery document could not be read.")
	}
	c.Set(fiber.HeaderCacheControl, "public, max-age=60")
	return c.JSON(doc)
}

func (d *Discovery) document(ctx context.Context) (map[string]any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cached != nil && time.Since(d.fetched) < discoveryTTL {
		return d.cached, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.engine, nil)
	if err != nil {
		return nil, err
	}
	res, err := d.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, &discoveryStatusError{res.StatusCode}
	}
	var engine map[string]any
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&engine); err != nil {
		return nil, err
	}
	d.cached, d.fetched = d.overlay(engine), time.Now()
	return d.cached, nil
}

// overlay is the authorization server's document as Authway offers it.
func (d *Discovery) overlay(engine map[string]any) map[string]any {
	doc := make(map[string]any, len(engine)+4)
	for k, v := range engine {
		doc[k] = v
	}
	// A client registers response type code only.
	doc["response_types_supported"] = []string{"code"}
	// Grants a client can be registered for: implicit is not one; token
	// exchange and approval polling are answered by Authway at the token
	// endpoint.
	grants := slices.DeleteFunc(stringList(engine["grant_types_supported"]), func(g string) bool { return g == "implicit" })
	if d.exchange {
		grants = append(grants, client.TokenExchangeGrant)
	}
	if d.approvals {
		grants = append(grants, delegation.ApprovalGrant)
	}
	doc["grant_types_supported"] = grants
	// A client is registered with client_secret_post, private_key_jwt or
	// none; the authorization server checks the registered method, so HTTP
	// Basic is refused.
	doc["token_endpoint_auth_methods_supported"] = []string{client.AuthMethodSecret, client.AuthMethodPrivateKeyJWT, client.AuthMethodNone}
	doc["token_endpoint_auth_signing_alg_values_supported"] = client.AssertionAlgs()
	// No place to register a front-channel logout address, nor request URIs
	// (which the authorization server requires registered).
	doc["frontchannel_logout_supported"] = false
	doc["frontchannel_logout_session_supported"] = false
	doc["request_uri_parameter_supported"] = false
	doc["introspection_endpoint"] = d.apiURL + "/api/v1/oauth2/introspect"
	doc["introspection_endpoint_auth_methods_supported"] = []string{"client_secret_basic", client.AuthMethodSecret}
	if d.approvals {
		// Authway's own: where a client acting for a user asks them to
		// approve one action (docs/api/delegation.md §5).
		doc["approval_request_endpoint"] = d.apiURL + "/oauth2/approval-requests"
	}
	return doc
}

func stringList(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

type discoveryStatusError struct{ status int }

func (e *discoveryStatusError) Error() string {
	return "authorization server discovery answered " + http.StatusText(e.status)
}
