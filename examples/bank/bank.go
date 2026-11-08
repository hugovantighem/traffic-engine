// Package bank is an example of another service: its actions, with no link to the shop. A system
// adds the links between them (see the tests of the engine).
package bank

import (
	"context"
	"net/http"
	"sync"

	engine "github.com/hugovantighem/traffic-engine"
)

// BalanceP is a balance to read.
type BalanceP struct{ Customer string }

// Account is the balance of a customer.
type Account struct {
	Customer string
	Cents    int64
}

// Ledger is the service, in memory.
type Ledger struct {
	mu       sync.Mutex
	balances map[string]int64
	// Asked are the customers whose balance was read, in order.
	Asked []string
}

// NewLedger returns an empty ledger.
func NewLedger() *Ledger { return &Ledger{balances: map[string]int64{}} }

// Credit adds to the balance of a customer.
func (l *Ledger) Credit(customer string, cents int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.balances[customer] += cents
}

// Actions are the actions of the bank.
type Actions struct {
	Balance *engine.Action[BalanceP, Account]
}

// New returns the actions of the bank on a ledger.
func New(l *Ledger) *Actions {
	return &Actions{Balance: &engine.Action[BalanceP, Account]{
		Name: "balance",
		Do: func(_ context.Context, p BalanceP, _ string) engine.Result[Account] {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.Asked = append(l.Asked, p.Customer)
			cents, ok := l.balances[p.Customer]
			if !ok {
				return engine.Result[Account]{Status: http.StatusNotFound}
			}
			return engine.Result[Account]{Status: http.StatusOK, Body: &Account{Customer: p.Customer, Cents: cents}}
		},
	}}
}
