# traffic-engine

A small traffic engine in Go: simulated users walk through the actions of a system, the way a
person would, and every step is journaled (status, duration, trace ID). No scenario to write by
hand: each action says what can follow it, and how to build the parameters of the next action
from what it sent and what it received.

```go
import engine "github.com/hugovantighem/traffic-engine"
```

## Actions and links

An action is one operation of an API, called with its generated client. `P` is its parameters, `R`
its success body:

```go
buy := &engine.Action[BuyP, Order]{
	Name:     "buy",
	Generate: func(r *rand.Rand) BuyP { return BuyP{Customer: pick(r), Cents: 100 + r.Int64N(1000)} },
	Do: func(ctx context.Context, p BuyP, traceparent string) engine.Result[Order] {
		resp, err := api.BuyWithResponse(ctx, client.BuyRequest{Customer: p.Customer, Cents: p.Cents}, traced(traceparent))
		if err != nil {
			return engine.Result[Order]{Err: err}
		}
		return engine.Result[Order]{Status: resp.StatusCode(), Body: resp.JSON201}
	},
}
buy.Then(engine.Follow(refund, func(_ *rand.Rand, _ BuyP, out Order) (RefundP, bool) {
	return RefundP{OrderID: out.ID}, true // typed: the compiler checks every field
}))
```

Deliberate errors (`Fail`, `FailStart`, a `Breaker` per wrong way to call) are played with a share
set by the picker, and must get the status they expect.

## One package per service, a graph per system

Each service publishes its actions and their own links (a package `traffic/` next to its
contract). A system imports them, **adds the links between services** with `Then`, without
redefining any action, and keeps in a `Graph` what it plays:

```go
shop.Buy.Then(engine.Follow(bank.Balance, func(_ *rand.Rand, _ shop.BuyP, out shop.Order) (bank.BalanceP, bool) {
	return bank.BalanceP{Customer: out.Customer}, true
}))
g := engine.NewGraph().Add(shop.Buy, bank.Balance).Start(engine.StartWith(shop.Buy, 1))
u := engine.NewUser("u1", seed, g.Config(engine.Config{Picker: engine.Weighted{StopProb: 0.2}}))
```

An action left out of the graph is never played, even when an action links to it: a system
leaves out what only another service calls. See `examples/` and `graph_test.go`.

## Coverage

`g.Cover(journals...)` tells which entry points and links were never played as expected, and
which steps went wrong (a technical error, a normal action that failed, a deliberate error that
did not get its status). A test that walks until everything is covered checks the system and its
traffic together: an action added without its links, or a contract change that breaks one,
fails it.
