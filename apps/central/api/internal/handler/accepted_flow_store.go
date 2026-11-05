package handler

import (
	"context"
	"time"

	"github.com/google/uuid"

	"authway/apps/central/api/pkg/ephemeral"
	"authway/apps/central/api/pkg/tokenhash"
)

// acceptedFlowTTL bounds how long a signed-in login flow is remembered. A
// repeated submit comes from a screen that has not yet followed the redirect —
// seconds, or a slow network's minutes — and the authorization server does not
// take the flow again once the browser has been through it, so a longer
// memory buys nothing.
const acceptedFlowTTL = 10 * time.Minute

// AcceptedFlowStore remembers which login flows have been accepted, as whom,
// and where the browser was sent.
//
// A sign-in screen stays live while the browser follows the redirect, so the
// same flow is submitted again by a second click or a retry on a slow network.
// The authorization server takes the flow again and answers again; without
// this memory every submit would count as another sign-in. With it, the same
// user proving who they are a second time is answered with the same redirect
// and nothing new is recorded, and anyone else is refused: a flow belongs to
// the user who signed in to it.
type AcceptedFlowStore struct {
	store  ephemeral.Store
	prefix string
}

func NewAcceptedFlowStore(store ephemeral.Store) *AcceptedFlowStore {
	return &AcceptedFlowStore{store: store, prefix: "flow_accepted:"}
}

// key names the flow by a hash: the flow id is what opens the sign-in screen
// and is not kept as a key name.
func (s *AcceptedFlowStore) key(flow string) string {
	return s.prefix + tokenhash.Hash(flow)
}

// Remember notes that flow was accepted for the user and sent to redirectTo.
func (s *AcceptedFlowStore) Remember(flow string, userID uuid.UUID, redirectTo string) {
	if s == nil || s.store == nil || flow == "" {
		return
	}
	s.store.Put(context.Background(), s.key(flow), map[string]string{
		"user_id":     userID.String(),
		"redirect_to": redirectTo,
	}, acceptedFlowTTL)
}

// Recall returns who flow was accepted for and where they were sent; ok is
// false for a flow not accepted (or not remembered any longer).
func (s *AcceptedFlowStore) Recall(flow string) (userID uuid.UUID, redirectTo string, ok bool) {
	if s == nil || s.store == nil || flow == "" {
		return uuid.Nil, "", false
	}
	fields, err := s.store.Fields(context.Background(), s.key(flow))
	if err != nil || len(fields) == 0 {
		return uuid.Nil, "", false
	}
	id, err := uuid.Parse(fields["user_id"])
	if err != nil || fields["redirect_to"] == "" {
		return uuid.Nil, "", false
	}
	return id, fields["redirect_to"], true
}

// acceptingTTL bounds how long one request may hold a flow while it has the
// authorization server accept it. It is only a ceiling for a request that dies
// holding it: the holder lets go as soon as it has an answer.
const acceptingTTL = 30 * time.Second

// acceptAwait is how long a request that finds its flow being accepted waits
// for the answer: the authorization server answers in milliseconds, and the
// request that gave up waiting accepts the flow itself.
const acceptAwait = 5 * time.Second

// acceptPoll is how often a waiting request looks again.
const acceptPoll = 25 * time.Millisecond

func (s *AcceptedFlowStore) accepting(flow string) string {
	return "flow_accepting:" + tokenhash.Hash(flow)
}

// Begin takes the right to have flow accepted. It is false when another
// request holds it: two submits of one flow that overlap (the second click
// comes before the first answer) must not both sign in.
func (s *AcceptedFlowStore) Begin(flow string) bool {
	if s == nil || s.store == nil || flow == "" {
		return true
	}
	first, err := s.store.MarkOnce(context.Background(), s.accepting(flow), acceptingTTL)
	// A store that cannot say is not a reason to refuse a sign-in.
	return err != nil || first
}

// End lets go of the flow, whether or not it was accepted.
func (s *AcceptedFlowStore) End(flow string) {
	if s == nil || s.store == nil || flow == "" {
		return
	}
	_ = s.store.Remove(context.Background(), s.accepting(flow))
}

// Await waits for the request that holds flow to say how it went: who the flow
// was accepted for and where they were sent, or ok false when it let go
// without accepting (it failed) or took too long.
func (s *AcceptedFlowStore) Await(flow string) (userID uuid.UUID, redirectTo string, ok bool) {
	deadline := time.Now().Add(acceptAwait)
	for {
		if id, to, found := s.Recall(flow); found {
			return id, to, true
		}
		if _, held, err := s.store.Remaining(context.Background(), s.accepting(flow)); err != nil || !held {
			// Let go: the answer, if there is one, was recorded before it.
			id, to, found := s.Recall(flow)
			return id, to, found
		}
		if time.Now().After(deadline) {
			return uuid.Nil, "", false
		}
		time.Sleep(acceptPoll)
	}
}
