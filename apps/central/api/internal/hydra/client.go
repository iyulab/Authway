package hydra

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrFlowNotFound is returned when Hydra does not know a login or consent
// challenge — the caller sent an id that was never issued or is malformed.
// ClientRefusedError is Hydra declining a client's registration because of
// what the registration says — as opposed to Hydra failing.
type ClientRefusedError struct {
	// Reason is Hydra's own account of what it would not accept.
	Reason string
}

func (e *ClientRefusedError) Error() string {
	return "the authorization server refused the client: " + e.Reason
}

func clientRefused(body []byte) *ClientRefusedError {
	var answer struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &answer)
	reason := answer.Description
	if reason == "" {
		reason = answer.Error
	}
	if reason == "" {
		reason = string(body)
	}
	return &ClientRefusedError{Reason: reason}
}

var ErrFlowNotFound = errors.New("login or consent flow not found")

// ErrFlowExpired is returned when Hydra knows the challenge but it can no
// longer be acted on — it was already accepted or rejected, or it timed out.
var ErrFlowExpired = errors.New("login or consent flow expired or already handled")

// ErrUserCodeRejected is returned when the authorization server does not
// accept the code a user typed for a device sign-in: it is not the code the
// device was shown, or the device's request has expired.
var ErrUserCodeRejected = errors.New("user code not accepted")

// flowError classifies a non-200 response to a call on a login, consent or
// logout challenge. The id comes from the caller, so calls fail for reasons
// the caller caused (unknown, used, expired), which must surface as client
// errors, not as Hydra being down.
func flowError(status int, body []byte) error {
	switch status {
	case http.StatusNotFound:
		return fmt.Errorf("%w: hydra returned %d", ErrFlowNotFound, status)
	case http.StatusGone:
		return fmt.Errorf("%w: hydra returned %d", ErrFlowExpired, status)
	default:
		return fmt.Errorf("hydra returned status %d: %s", status, string(body))
	}
}

type Client struct {
	AdminURL string
	client   *http.Client
}

func NewClient(adminURL string) *Client {
	return &Client{
		AdminURL: adminURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Don't follow redirects - return error to stop redirect following
				// This prevents PUT requests from being converted to GET on redirect
				return http.ErrUseLastResponse
			},
		},
	}
}

// OAuth2 Client Management
type OAuth2Client struct {
	ClientID               string   `json:"client_id"`
	ClientName             string   `json:"client_name"`
	ClientSecret           string   `json:"client_secret,omitempty"`
	RedirectUris           []string `json:"redirect_uris"`
	PostLogoutRedirectUris []string `json:"post_logout_redirect_uris,omitempty"`
	// Audience is the resource servers the client's access tokens may be
	// for. Not omitempty: an update replaces the whole client, and an empty
	// list has to reach Hydra to clear one set earlier.
	Audience []string `json:"audience"`
	// BackChannelLogoutURI is where the client is told that a session ended
	// (OpenID Connect Back-Channel Logout). Omitted when empty, which on an
	// update clears it.
	BackChannelLogoutURI string `json:"backchannel_logout_uri,omitempty"`
	// ClientURI is the application's home page (OpenID Connect Dynamic
	// Client Registration).
	ClientURI string `json:"client_uri,omitempty"`
	// LogoURI is the application's logo (OpenID Connect Dynamic Client
	// Registration), which the sign-in and consent screens show.
	LogoURI                          string   `json:"logo_uri,omitempty"`
	BackChannelLogoutSessionRequired bool     `json:"backchannel_logout_session_required,omitempty"`
	GrantTypes                       []string `json:"grant_types"`
	ResponseTypes                    []string `json:"response_types"`
	Scope                            string   `json:"scope"`
	TokenEndpointAuthMethod          string   `json:"token_endpoint_auth_method"`
	// JWKS / JWKSURI are the public keys of a client that authenticates with
	// a signed assertion (private_key_jwt). Omitted when empty, which on an
	// update clears them.
	JWKS    json.RawMessage `json:"jwks,omitempty"`
	JWKSURI string          `json:"jwks_uri,omitempty"`

	// Consent flow configuration (Hydra admin-API-only fields).
	// Deliberately NOT omitempty: the Authway DB is the source of truth, so a
	// false value must be transmitted to overwrite a previously-true state in
	// Hydra (e.g. when an admin toggles skip_consent back off, or on secret
	// regeneration which re-sends the full client object).
	SkipConsent       bool `json:"skip_consent"`
	SkipLogoutConsent bool `json:"skip_logout_consent"`

	// Per-client access token format ("jwt" | "opaque"), overriding Hydra's
	// global `strategies.access_token`. omitempty is correct here — unlike the
	// booleans above, the zero value *is* the "inherit the global setting" state,
	// and client update is a PUT full-replace, so omitting the field resets a
	// previously-pinned client back to inheriting.
	AccessTokenStrategy string `json:"access_token_strategy,omitempty"`
}

func (c *Client) CreateOAuth2Client(client *OAuth2Client) (*OAuth2Client, error) {
	data, err := json.Marshal(client)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Post(
		fmt.Sprintf("%s/admin/clients", c.AdminURL),
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusBadRequest {
			return nil, clientRefused(body)
		}
		return nil, fmt.Errorf("failed to create client: %s", string(body))
	}

	var result OAuth2Client
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) GetOAuth2Client(clientID string) (*OAuth2Client, error) {
	resp, err := c.client.Get(
		fmt.Sprintf("%s/admin/clients/%s", c.AdminURL, clientID),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("client not found")
	}

	var client OAuth2Client
	if err := json.NewDecoder(resp.Body).Decode(&client); err != nil {
		return nil, err
	}

	return &client, nil
}

func (c *Client) UpdateOAuth2Client(clientID string, client *OAuth2Client) (*OAuth2Client, error) {
	data, err := json.Marshal(client)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/clients/%s", c.AdminURL, clientID),
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// As on registration: what the update says was refused.
		if resp.StatusCode == http.StatusBadRequest {
			return nil, clientRefused(body)
		}
		return nil, fmt.Errorf("failed to update client: %s", string(body))
	}

	var result OAuth2Client
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateClient performs a partial update on an OAuth2 client using PATCH semantics
// It first fetches the current client, merges the updates, then performs a PUT
func (c *Client) UpdateClient(clientID string, updates map[string]any) error {
	// First, get the current client
	currentClient, err := c.GetOAuth2Client(clientID)
	if err != nil {
		return fmt.Errorf("failed to get current client: %w", err)
	}

	// Convert current client to map for merging
	clientData, err := json.Marshal(currentClient)
	if err != nil {
		return fmt.Errorf("failed to marshal current client: %w", err)
	}

	var clientMap map[string]any
	if err := json.Unmarshal(clientData, &clientMap); err != nil {
		return fmt.Errorf("failed to unmarshal current client: %w", err)
	}

	// Merge updates into client map
	for key, value := range updates {
		clientMap[key] = value
	}

	// Send the merged update
	data, err := json.Marshal(clientMap)
	if err != nil {
		return fmt.Errorf("failed to marshal updated client: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/clients/%s", c.AdminURL, clientID),
		bytes.NewBuffer(data),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update client: %s", string(body))
	}

	return nil
}

func (c *Client) DeleteOAuth2Client(clientID string) error {
	req, err := http.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("%s/admin/clients/%s", c.AdminURL, clientID),
		nil,
	)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete client: %s", string(body))
	}

	return nil
}

// Login and Consent Flow Management
type LoginRequest struct {
	Challenge         string        `json:"challenge"`
	RequestedScope    []string      `json:"requested_scope"`
	RequestedAudience []string      `json:"requested_access_token_audience"`
	Subject           string        `json:"subject"`
	Client            *OAuth2Client `json:"client"`
	RequestURL        string        `json:"request_url"`
	SessionID         string        `json:"session_id"`
	Skip              bool          `json:"skip"`
	// OIDCContext is what the authorization request said about how the user
	// should be signed in (OpenID Connect Core §3.1.2.1).
	OIDCContext *OIDCContext `json:"oidc_context,omitempty"`
}

// OIDCContext carries the optional OpenID Connect parameters of an
// authorization request that concern the sign-in screen.
type OIDCContext struct {
	LoginHint string   `json:"login_hint,omitempty"`
	UILocales []string `json:"ui_locales,omitempty"`
}

type AcceptLoginRequest struct {
	Subject     string `json:"subject"`
	Remember    bool   `json:"remember"`
	RememberFor int    `json:"remember_for"`
	ACR         string `json:"acr,omitempty"`
	// AMR is how the user authenticated; the authorization server puts it in
	// the ID token as the amr claim.
	AMR     []string       `json:"amr,omitempty"`
	Context map[string]any `json:"context,omitempty"`
}

type LoginResponse struct {
	RedirectTo string `json:"redirect_to"`
}

func (c *Client) GetLoginRequest(challenge string) (*LoginRequest, error) {
	url := fmt.Sprintf("%s/admin/oauth2/auth/requests/login?challenge=%s", c.AdminURL, url.QueryEscape(challenge))
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to request Hydra at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, flowError(resp.StatusCode, body)
	}

	var loginReq LoginRequest
	if err := json.NewDecoder(resp.Body).Decode(&loginReq); err != nil {
		return nil, fmt.Errorf("failed to decode login request: %w", err)
	}

	return &loginReq, nil
}

func (c *Client) AcceptLoginRequest(challenge string, body *AcceptLoginRequest) (*LoginResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/oauth2/auth/requests/login/accept?challenge=%s", c.AdminURL, url.QueryEscape(challenge)),
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body for proper error handling
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra accept login: %w", flowError(resp.StatusCode, bodyBytes))
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		return nil, fmt.Errorf("failed to decode login response: %w, body: %s", err, string(bodyBytes))
	}

	return &loginResp, nil
}

func (c *Client) RejectLoginRequest(challenge string, error_code, error_description string) (*LoginResponse, error) {
	body := map[string]string{
		"error":             error_code,
		"error_description": error_description,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/oauth2/auth/requests/login/reject?challenge=%s", c.AdminURL, url.QueryEscape(challenge)),
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body for proper error handling
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra reject login: %w", flowError(resp.StatusCode, bodyBytes))
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		return nil, fmt.Errorf("failed to decode login response: %w, body: %s", err, string(bodyBytes))
	}

	return &loginResp, nil
}

// Consent Flow
type ConsentRequest struct {
	Challenge         string         `json:"challenge"`
	RequestedScope    []string       `json:"requested_scope"`
	RequestedAudience []string       `json:"requested_access_token_audience"`
	Subject           string         `json:"subject"`
	Client            *OAuth2Client  `json:"client"`
	LoginChallenge    string         `json:"login_challenge"`
	LoginSessionID    string         `json:"login_session_id"`
	ACR               string         `json:"acr"`
	Context           map[string]any `json:"context"`
	Skip              bool           `json:"skip"`
}

type AcceptConsentRequest struct {
	GrantScope               []string        `json:"grant_scope"`
	GrantAccessTokenAudience []string        `json:"grant_access_token_audience,omitempty"`
	Remember                 bool            `json:"remember"`
	RememberFor              int             `json:"remember_for"`
	Session                  *ConsentSession `json:"session,omitempty"`
}

type ConsentSession struct {
	AccessToken map[string]any `json:"access_token,omitempty"`
	IDToken     map[string]any `json:"id_token,omitempty"`
}

func (c *Client) GetConsentRequest(challenge string) (*ConsentRequest, error) {
	resp, err := c.client.Get(
		fmt.Sprintf("%s/admin/oauth2/auth/requests/consent?challenge=%s", c.AdminURL, url.QueryEscape(challenge)),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body for proper error handling
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, flowError(resp.StatusCode, bodyBytes)
	}

	var consentReq ConsentRequest
	if err := json.Unmarshal(bodyBytes, &consentReq); err != nil {
		return nil, fmt.Errorf("failed to decode consent request: %w, body: %s", err, string(bodyBytes))
	}

	return &consentReq, nil
}

func (c *Client) AcceptConsentRequest(challenge string, body *AcceptConsentRequest) (*LoginResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/oauth2/auth/requests/consent/accept?challenge=%s", c.AdminURL, url.QueryEscape(challenge)),
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body for proper error handling
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra accept consent: %w", flowError(resp.StatusCode, bodyBytes))
	}

	var consentResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &consentResp); err != nil {
		return nil, fmt.Errorf("failed to decode consent response: %w, body: %s", err, string(bodyBytes))
	}

	return &consentResp, nil
}

func (c *Client) RejectConsentRequest(challenge string, error_code, error_description string) (*LoginResponse, error) {
	body := map[string]string{
		"error":             error_code,
		"error_description": error_description,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/oauth2/auth/requests/consent/reject?challenge=%s", c.AdminURL, url.QueryEscape(challenge)),
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body for proper error handling
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra reject consent: %w", flowError(resp.StatusCode, bodyBytes))
	}

	var consentResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &consentResp); err != nil {
		return nil, fmt.Errorf("failed to decode consent response: %w, body: %s", err, string(bodyBytes))
	}

	return &consentResp, nil
}

// Session Management
// ErrApplicationsNotNotified reports that a user's sessions were ended but the
// applications signed in on them could not all be told (back-channel logout).
// The sessions are gone either way; callers treat it as a warning.
var ErrApplicationsNotNotified = errors.New("sessions ended, but not every application could be notified")

// RevokeUserSessions ends every session of a user and tells the applications
// that were signed in on them.
//
// Ending the sessions of a subject in one call is silent: the authorization
// server sends a logout token to an application's backchannel_logout_uri only
// when a session is ended by its id (measured against v26.2.0). So the
// subject's sessions are listed and ended one by one first; the by-subject
// calls that follow remove whatever is left and the remembered consents.
//
// The subject is URL-escaped, so an identifier containing reserved characters
// is transmitted intact.
func (c *Client) RevokeUserSessions(subject string) error {
	escapedSubject := url.QueryEscape(subject)

	var notifyErr error
	sessionIDs, err := c.loginSessionIDs(escapedSubject)
	if err != nil {
		notifyErr = err
	}
	for _, id := range sessionIDs {
		if err := c.deleteSessions("login?sid=" + url.QueryEscape(id)); err != nil {
			notifyErr = err
		}
	}

	if err := c.deleteSessions("login?subject=" + escapedSubject); err != nil {
		return fmt.Errorf("failed to revoke login sessions: %w", err)
	}
	if err := c.deleteSessions("consent?subject=" + escapedSubject + "&all=true"); err != nil {
		return fmt.Errorf("failed to revoke consent sessions: %w", err)
	}
	if notifyErr != nil {
		return fmt.Errorf("%w: %v", ErrApplicationsNotNotified, notifyErr)
	}
	return nil
}

// RevokeOtherSessions ends every session of a user except the one named, and
// tells the applications that were signed in on the others. With keep empty
// it is RevokeUserSessions: there is no session to tell apart.
//
// What ends a session's tokens is revoking the consent granted in it, named
// by its own id; deleting the login session alone leaves its access and
// refresh tokens working (both measured against v26.2.0). A consent names the
// login session it was granted in, so the consents of other sessions are
// revoked one by one and their login sessions ended by id, which is also what
// sends the logout token. The consents are read first: ending a login session
// blanks the session id on its consents.
func (c *Client) RevokeOtherSessions(subject, keep string) error {
	if keep == "" {
		return c.RevokeUserSessions(subject)
	}
	consents, err := c.consentSessions(url.QueryEscape(subject))
	if err != nil {
		return err
	}
	var firstErr error
	ended := map[string]bool{}
	for _, consent := range consents {
		if consent.LoginSessionID == keep {
			continue
		}
		if id := consent.LoginSessionID; id != "" && !ended[id] {
			ended[id] = true
			if err := c.deleteSessions("login?sid=" + url.QueryEscape(id)); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("%w: %v", ErrApplicationsNotNotified, err)
			}
		}
		if err := c.deleteSessions("consent?consent_request_id=" + url.QueryEscape(consent.RequestID)); err != nil {
			return fmt.Errorf("failed to revoke a consent session: %w", err)
		}
	}
	return firstErr
}

// consentSession is a consent a user granted: its own id and the login
// session it was granted in (empty once that session has ended).
type consentSession struct {
	RequestID      string
	LoginSessionID string
}

// consentSessions lists the consents a subject has granted.
func (c *Client) consentSessions(escapedSubject string) ([]consentSession, error) {
	resp, err := c.client.Get(c.AdminURL + "/admin/oauth2/auth/sessions/consent?page_size=500&subject=" + escapedSubject)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list sessions: status=%d body=%s", resp.StatusCode, string(body))
	}
	var listed []struct {
		RequestID      string `json:"consent_request_id"`
		ConsentRequest struct {
			LoginSessionID string `json:"login_session_id"`
		} `json:"consent_request"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		return nil, fmt.Errorf("failed to decode sessions: %w", err)
	}
	sessions := make([]consentSession, 0, len(listed))
	for _, l := range listed {
		sessions = append(sessions, consentSession{RequestID: l.RequestID, LoginSessionID: l.ConsentRequest.LoginSessionID})
	}
	return sessions, nil
}

// deleteSessions calls DELETE /admin/oauth2/auth/sessions/<pathAndQuery>.
// Nothing to delete is not a failure.
func (c *Client) deleteSessions(pathAndQuery string) error {
	req, err := http.NewRequest(http.MethodDelete, c.AdminURL+"/admin/oauth2/auth/sessions/"+pathAndQuery, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

// loginSessionIDs returns the ids of the login sessions the subject has used
// with an application. The authorization server offers no list of login
// sessions; the consent sessions name the login session each was granted in.
func (c *Client) loginSessionIDs(escapedSubject string) ([]string, error) {
	resp, err := c.client.Get(c.AdminURL + "/admin/oauth2/auth/sessions/consent?page_size=500&subject=" + escapedSubject)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list sessions: status=%d body=%s", resp.StatusCode, string(body))
	}
	var consents []struct {
		ConsentRequest struct {
			LoginSessionID string `json:"login_session_id"`
		} `json:"consent_request"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&consents); err != nil {
		return nil, fmt.Errorf("failed to read the session list: %w", err)
	}
	seen := map[string]bool{}
	var ids []string
	for _, consent := range consents {
		if id := consent.ConsentRequest.LoginSessionID; id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// Token Introspection
type IntrospectRequest struct {
	Token string `json:"token"`
	Scope string `json:"scope,omitempty"`
}

type IntrospectResponse struct {
	Active    bool           `json:"active"`
	Subject   string         `json:"sub,omitempty"`
	ClientID  string         `json:"client_id,omitempty"`
	Scope     string         `json:"scope,omitempty"`
	TokenType string         `json:"token_type,omitempty"`
	TokenUse  string         `json:"token_use,omitempty"`
	Issuer    string         `json:"iss,omitempty"`
	Audience  []string       `json:"aud,omitempty"`
	Exp       int64          `json:"exp,omitempty"`
	Iat       int64          `json:"iat,omitempty"`
	Nbf       int64          `json:"nbf,omitempty"`
	Ext       map[string]any `json:"ext,omitempty"`
}

func (c *Client) IntrospectToken(token string) (*IntrospectResponse, error) {
	// OAuth 2.0 Token Introspection requires form-encoded data
	formData := url.Values{}
	formData.Set("token", token)

	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/admin/oauth2/introspect", c.AdminURL),
		strings.NewReader(formData.Encode()),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("introspection failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var introspectResp IntrospectResponse
	if err := json.Unmarshal(bodyBytes, &introspectResp); err != nil {
		return nil, fmt.Errorf("failed to decode introspection response: %w, body: %s", err, string(bodyBytes))
	}

	return &introspectResp, nil
}

// LogoutRequest is Hydra's view of an RP- or browser-initiated logout.
type LogoutRequest struct {
	Challenge   string `json:"challenge"`
	Subject     string `json:"subject"`
	SessionID   string `json:"sid"`
	RequestURL  string `json:"request_url"`
	RPInitiated bool   `json:"rp_initiated"`
	Client      *struct {
		ClientID string `json:"client_id"`
	} `json:"client,omitempty"`
}

// GetLogoutRequest fetches a logout request. Unknown and already-handled
// challenges are reported as ErrFlowNotFound / ErrFlowExpired.
func (c *Client) GetLogoutRequest(challenge string) (*LogoutRequest, error) {
	resp, err := c.client.Get(fmt.Sprintf("%s/admin/oauth2/auth/requests/logout?logout_challenge=%s",
		c.AdminURL, url.QueryEscape(challenge)))
	if err != nil {
		return nil, fmt.Errorf("failed to request Hydra: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, flowError(resp.StatusCode, body)
	}
	var req LogoutRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to decode logout request: %w", err)
	}
	return &req, nil
}

// AcceptUserCodeRequest confirms the code a user typed on the device sign-in
// page (RFC 8628) and returns where the browser goes next — on to the
// ordinary login and consent requests for the device's client.
func (c *Client) AcceptUserCodeRequest(challenge, userCode string) (*LoginResponse, error) {
	data, err := json.Marshal(map[string]string{"user_code": userCode})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/admin/oauth2/auth/requests/device/accept?device_challenge=%s",
		c.AdminURL, url.QueryEscape(challenge)), bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to accept user code: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusBadRequest {
		return nil, fmt.Errorf("%w: hydra returned %d", ErrUserCodeRejected, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, flowError(resp.StatusCode, respBody)
	}
	var out LoginResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("failed to decode user code accept response: %w", err)
	}
	return &out, nil
}

// AcceptLogoutRequest accepts a logout request and returns where Hydra wants
// the browser to go next. The request carries no body: Hydra decides the
// post-logout destination itself (the RP's validated post_logout_redirect_uri,
// or its configured default).
func (c *Client) AcceptLogoutRequest(challenge string) (*LoginResponse, error) {
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/admin/oauth2/auth/requests/logout/accept?logout_challenge=%s",
		c.AdminURL, url.QueryEscape(challenge)), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to accept logout: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, flowError(resp.StatusCode, respBody)
	}
	var out LoginResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("failed to decode logout accept response: %w", err)
	}
	return &out, nil
}
