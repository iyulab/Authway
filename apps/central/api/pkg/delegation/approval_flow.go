package delegation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ApprovalGrant is the grant a client polls an approval with — Authway's own
// name: the flow is not OpenID CIBA (the token carries no ID token).
const ApprovalGrant = "urn:authway:params:oauth:grant-type:approval"

// Approval request limits.
const (
	approvalDefaultExpiry = 300 * time.Second
	approvalMinExpiry     = 30 * time.Second
	approvalMaxExpiry     = 600 * time.Second
	maxBindingMessage     = 200
	maxAuthorizationBytes = 4096
)

// ApprovalRequested is the answer to an approval request.
type ApprovalRequested struct {
	AuthReqID  string `json:"auth_req_id"`
	ApproveURL string `json:"approve_url"`
	ExpiresIn  int    `json:"expires_in"`
	Interval   int    `json:"interval"`
	// Request is the stored request, for the caller's audit record.
	Request *Approval `json:"-"`
}

// RequestApproval checks an approval request — the client's own credentials,
// a delegation of the user to it, what it asks for — and records it.
func (x *Exchanger) RequestApproval(ctx context.Context, form url.Values, clientID, secret string) (*ApprovalRequested, *Standing, *OAuthError) {
	if !x.OffersApprovals() {
		return nil, nil, refuse(404, "not_found", "Approval requests are not offered by this deployment.")
	}
	row, ok := x.verifier.VerifyClient(clientID, secret)
	if !ok {
		return nil, nil, refuse(401, "invalid_client", "Authenticate as a confidential client: its id and secret, as HTTP Basic or in the form.")
	}
	if form.Get("subject_token") == "" || form.Get("subject_token_type") != SubjectTokenType {
		return nil, nil, refuse(400, "invalid_request", "subject_token must be a delegation handle (subject_token_type "+SubjectTokenType+").")
	}
	binding := strings.TrimSpace(form.Get("binding_message"))
	if binding == "" || utf8.RuneCountInString(binding) > maxBindingMessage {
		return nil, nil, refuse(400, "invalid_request", fmt.Sprintf("binding_message is required and at most %d characters: it is what the user is shown.", maxBindingMessage))
	}
	var details json.RawMessage
	if raw := form.Get("authorization_details"); raw != "" {
		var list []map[string]any
		if len(raw) > maxAuthorizationBytes || json.Unmarshal([]byte(raw), &list) != nil || len(list) == 0 {
			return nil, nil, refuse(400, "invalid_authorization_details", fmt.Sprintf("authorization_details must be a JSON array of objects (RFC 9396), at most %d bytes.", maxAuthorizationBytes))
		}
		for _, d := range list {
			if t, _ := d["type"].(string); t == "" {
				return nil, nil, refuse(400, "invalid_authorization_details", "Every authorization_details entry names its type.")
			}
		}
		details = json.RawMessage(raw)
	}
	expiry := approvalDefaultExpiry
	if v := form.Get("requested_expiry"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || time.Duration(secs)*time.Second < approvalMinExpiry || time.Duration(secs)*time.Second > approvalMaxExpiry {
			return nil, nil, refuse(400, "invalid_request", fmt.Sprintf("requested_expiry is a number of seconds from %d to %d.", int(approvalMinExpiry.Seconds()), int(approvalMaxExpiry.Seconds())))
		}
		expiry = time.Duration(secs) * time.Second
	}

	d, err := x.delegations.Resolve(ctx, form.Get("subject_token"))
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, nil, refuse(400, "invalid_grant", "The delegation does not exist or was revoked.")
	case err != nil:
		return nil, nil, refuse(500, "server_error", "The delegation could not be read.")
	}
	if d.ClientID != clientID {
		return nil, d, refuse(400, "invalid_grant", "The delegation does not exist or was revoked.")
	}
	if why := d.usable(); why != "" {
		return nil, d, refuse(400, "invalid_grant", "The delegation cannot be used now: "+why+".")
	}
	clientScopes, _, mayAct, err := x.clients.ExchangeScopes(clientID)
	if err != nil {
		return nil, d, refuse(500, "server_error", "The client could not be read.")
	}
	if !mayAct {
		return nil, d, refuse(400, "unauthorized_client", "The client is not registered for token exchange.")
	}
	ceiling := intersect(d.Scopes, clientScopes)
	scopes := ceiling
	if requested := strings.Fields(form.Get("scope")); len(requested) > 0 {
		for _, s := range requested {
			if !slices.Contains(ceiling, s) {
				return nil, d, refuse(400, "invalid_scope", fmt.Sprintf("%q is not within what the user delegated.", s))
			}
		}
		scopes = requested
	}

	secretID, r, err := x.approvals.Create(ctx, d, row, binding, scopes, details, expiry)
	if err != nil {
		return nil, d, refuse(500, "server_error", "The approval request could not be recorded.")
	}
	return &ApprovalRequested{
		AuthReqID:  secretID,
		ApproveURL: x.approveURL + "?request=" + url.QueryEscape(r.ID.String()),
		ExpiresIn:  int(expiry.Seconds()),
		Interval:   int(ApprovalPollInterval.Seconds()),
		Request:    r,
	}, d, nil
}

// PrepareApproved answers a poll of an approval: pending, too soon, denied,
// expired — or, once approved, the assertion grant request for the token
// bound to it (the token hook consumes the approval when the token is
// issued).
func (x *Exchanger) PrepareApproved(ctx context.Context, form url.Values, clientID, secret string) (*Exchanged, *OAuthError) {
	if !x.OffersApprovals() {
		return nil, refuse(400, "unsupported_grant_type", "Approval requests are not offered by this deployment.")
	}
	row, ok := x.verifier.VerifyClient(clientID, secret)
	if !ok {
		return nil, refuse(401, "invalid_client", "Authenticate as a confidential client: its id and secret, as HTTP Basic or in the form.")
	}
	if form.Get("auth_req_id") == "" {
		return nil, refuse(400, "invalid_request", "auth_req_id is required.")
	}
	polled, err := x.approvals.Poll(ctx, form.Get("auth_req_id"), row)
	switch {
	case errors.Is(err, ErrApprovalNotFound):
		return nil, refuse(400, "invalid_grant", "The approval request does not exist.")
	case err != nil:
		return nil, refuse(500, "server_error", "The approval request could not be read.")
	}
	switch polled.State {
	case "expired":
		return nil, refuse(400, "expired_token", "The user did not decide in time.")
	case ApprovalDenied:
		return nil, refuse(400, "access_denied", "The user denied the request.")
	case "consumed":
		return nil, refuse(400, "invalid_grant", "The token of this approval was issued already.")
	case ApprovalPending:
		if polled.TooSoon {
			return nil, refuse(400, "slow_down", fmt.Sprintf("Ask at most every %d seconds.", int(ApprovalPollInterval.Seconds())))
		}
		return nil, refuse(400, "authorization_pending", "The user has not decided yet.")
	}

	r := polled.Approval
	d, err := x.delegations.Standing(ctx, r.DelegationID)
	if err != nil {
		return nil, refuse(400, "invalid_grant", "The delegation behind the approval was revoked.")
	}
	known := &Exchanged{Delegation: d}
	if why := d.usable(); why != "" {
		return known, refuse(400, "invalid_grant", "The delegation cannot be used now: "+why+".")
	}
	clientScopes, _, mayAct, err := x.clients.ExchangeScopes(clientID)
	if err != nil || !mayAct {
		return known, refuse(400, "unauthorized_client", "The client is not registered for token exchange.")
	}
	out, refused := x.engineForm(d, clientID, clientScopes, []string(r.Scopes), nil, form, r.ID.String())
	if refused != nil {
		return known, refused
	}
	return &Exchanged{Form: out, Delegation: d}, nil
}
