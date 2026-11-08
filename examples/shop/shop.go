// Package shop is an example of what a service publishes: its actions and their own links. It
// knows nothing of the other services; a system links them (see the tests of the engine).
//
// The "service" is a store in memory: a real one calls its API with the client generated from
// its contract.
package shop

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"

	engine "github.com/hugovantighem/traffic-engine"
)

// BuyP is an order to place.
type BuyP struct {
	Customer string
	Cents    int64
}

// Order is a placed order.
type Order struct {
	ID       string
	Customer string
	Cents    int64
}

// RefundP is an order to refund.
type RefundP struct{ OrderID string }

// Store is the service, in memory. OnBuy, when set, is told of each order (a system wires it to
// another service).
type Store struct {
	mu     sync.Mutex
	orders map[string]Order
	OnBuy  func(Order)
}

// NewStore returns an empty store.
func NewStore() *Store { return &Store{orders: map[string]Order{}} }

// Actions are the actions of the shop.
type Actions struct {
	Buy    *engine.Action[BuyP, Order]
	Refund *engine.Action[RefundP, Order]
}

// New returns the actions of the shop on a store, with their links: after an order, refund it or
// buy again.
func New(s *Store) *Actions {
	a := &Actions{
		Buy: &engine.Action[BuyP, Order]{
			Name: "buy",
			Generate: func(r *rand.Rand) BuyP {
				return BuyP{Customer: fmt.Sprintf("c%d", r.IntN(5)), Cents: 100 + r.Int64N(1000)}
			},
			Do: func(_ context.Context, p BuyP, _ string) engine.Result[Order] {
				o := s.buy(p)
				return engine.Result[Order]{Status: http.StatusCreated, Body: &o}
			},
		},
		Refund: &engine.Action[RefundP, Order]{
			Name: "refund",
			Do: func(_ context.Context, p RefundP, _ string) engine.Result[Order] {
				o, ok := s.order(p.OrderID)
				if !ok {
					return engine.Result[Order]{Status: http.StatusNotFound}
				}
				return engine.Result[Order]{Status: http.StatusOK, Body: &o}
			},
		},
	}
	a.Buy.Then(
		engine.Follow(a.Refund, func(_ *rand.Rand, _ BuyP, out Order) (RefundP, bool) { return RefundP{OrderID: out.ID}, true }),
		engine.Follow(a.Buy, func(r *rand.Rand, in BuyP, _ Order) (BuyP, bool) {
			return BuyP{Customer: in.Customer, Cents: 100 + r.Int64N(1000)}, true
		}),
	)
	return a
}

func (s *Store) buy(p BuyP) Order {
	s.mu.Lock()
	o := Order{ID: fmt.Sprintf("o%d", len(s.orders)+1), Customer: p.Customer, Cents: p.Cents}
	s.orders[o.ID] = o
	s.mu.Unlock()
	if s.OnBuy != nil {
		s.OnBuy(o)
	}
	return o
}

func (s *Store) order(id string) (Order, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	return o, ok
}
