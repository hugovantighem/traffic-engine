package engine_test

import (
	"context"
	"math/rand/v2"
	"slices"
	"testing"

	engine "github.com/hugovantighem/traffic-engine"
	"github.com/hugovantighem/traffic-engine/examples/bank"
	"github.com/hugovantighem/traffic-engine/examples/shop"
)

// system is a system made of two services published by two packages, linked here, from a third
// package, without redefining their actions: after an order, its customer looks at its balance
// at the bank, then may buy again.
type system struct {
	shop   *shop.Actions
	bank   *bank.Actions
	ledger *bank.Ledger
	graph  *engine.Graph
}

func givenSystem(keepRefund bool) system {
	store, ledger := shop.NewStore(), bank.NewLedger()
	store.OnBuy = func(o shop.Order) { ledger.Credit(o.Customer, o.Cents) }
	s := system{shop: shop.New(store), bank: bank.New(ledger), ledger: ledger}

	// The links between the services, added to actions they did not declare.
	s.shop.Buy.Then(engine.Follow(s.bank.Balance, func(_ *rand.Rand, _ shop.BuyP, out shop.Order) (bank.BalanceP, bool) {
		return bank.BalanceP{Customer: out.Customer}, true
	}))
	s.bank.Balance.Then(engine.Follow(s.shop.Buy, func(r *rand.Rand, _ bank.BalanceP, out bank.Account) (shop.BuyP, bool) {
		return shop.BuyP{Customer: out.Customer, Cents: 100 + r.Int64N(1000)}, true
	}))

	s.graph = engine.NewGraph().Add(s.shop.Buy, s.bank.Balance).Start(engine.StartWith(s.shop.Buy, 1))
	if keepRefund {
		s.graph.Add(s.shop.Refund)
	}
	return s
}

func walk(t *testing.T, s system, picker engine.Picker, walks int) [][]engine.Record {
	t.Helper()
	var journals [][]engine.Record
	for i := range walks {
		u := engine.NewUser("u", uint64(i), s.graph.Config(engine.Config{Picker: picker, MaxSteps: 10}))
		if err := u.Walk(context.Background()); err != nil {
			t.Fatal(err)
		}
		journals = append(journals, u.Journal)
	}
	return journals
}

func played(journals [][]engine.Record) []string {
	var out []string
	for _, j := range journals {
		for _, r := range j {
			if !slices.Contains(out, r.Action) {
				out = append(out, r.Action)
			}
		}
	}
	return out
}

func Test_Graph_AnActionNoLinkLeadsTo_IsUnreachable(t *testing.T) {
	// GIVEN the bank's balance in the graph, but no link to it (the system forgot it)
	store, ledger := shop.NewStore(), bank.NewLedger()
	sh, bk := shop.New(store), bank.New(ledger)
	g := engine.NewGraph().Add(sh.Buy, sh.Refund, bk.Balance).Start(engine.StartWith(sh.Buy, 1))

	// WHEN the graph is checked, without playing anything
	unreachable := g.Unreachable()

	// THEN the balance is named
	if !slices.Equal(unreachable, []string{"balance"}) {
		t.Fatalf("unreachable = %v", unreachable)
	}
}

func Test_Graph_ALinkBetweenServicesNeverPlayed_IsReportedMissing(t *testing.T) {
	// GIVEN the system, and users who only ever buy
	s := givenSystem(false)

	// WHEN they walk
	journals := walk(t, s, engine.Scripted{"buy", "buy"}, 3)

	// THEN the coverage names the links between services that were never played
	c := s.graph.Cover(journals...)
	if !slices.Contains(c.Missing, "buy -> balance") || !slices.Contains(c.Missing, "balance -> buy") {
		t.Fatalf("missing = %v", c.Missing)
	}
	if len(c.Broken) != 0 {
		t.Fatalf("broken = %v", c.Broken)
	}
}

func Test_Graph_AnActionLeftOut_IsNeverPlayed(t *testing.T) {
	// GIVEN the system without the refund, which the shop links after each order
	s := givenSystem(false)

	// WHEN users walk at random, many times
	journals := walk(t, s, engine.Weighted{StopProb: 0.1}, 200)

	// THEN no refund is ever played, and the graph does not ask for it
	if slices.Contains(played(journals), "refund") {
		t.Fatal("a refund was played")
	}
	for _, n := range s.graph.Nodes() {
		for _, l := range n.Links {
			if l.To == "refund" {
				t.Fatalf("%s links to the refund in the graph", n.Name)
			}
		}
	}
}

func Test_Graph_ALinkAddedFromAnotherPackage_IsFollowedWithItsValues(t *testing.T) {
	// GIVEN the system
	s := givenSystem(false)

	// WHEN a user buys, then looks at its balance
	journals := walk(t, s, engine.Scripted{"buy", "balance"}, 1)

	// THEN the bank was asked the balance of the customer who bought, and found it
	j := journals[0]
	if len(j) != 2 || j[1].Action != "balance" || j[1].Status != 200 {
		t.Fatalf("journal = %+v", j)
	}
	bought := j[0].Params.(shop.BuyP).Customer
	if len(s.ledger.Asked) != 1 || s.ledger.Asked[0] != bought {
		t.Fatalf("asked %v, bought by %s", s.ledger.Asked, bought)
	}
}

func Test_Graph_UsersCoverEverythingDeclared(t *testing.T) {
	// GIVEN the system with the refund, every action of it reachable
	s := givenSystem(true)
	if u := s.graph.Unreachable(); len(u) != 0 {
		t.Fatalf("unreachable: %v", u)
	}

	// WHEN users walk at random
	journals := walk(t, s, engine.Weighted{StopProb: 0.1}, 100)

	// THEN every entry point and every link was played, and nothing went wrong
	if c := s.graph.Cover(journals...); !c.OK() {
		t.Fatalf("missing %v, broken %v", c.Missing, c.Broken)
	}
}
