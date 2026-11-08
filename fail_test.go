package engine_test

import (
	"context"
	"math/rand/v2"
	"testing"

	"github.com/hugovantighem/traffic-engine"
)

type page struct{ IDs []int }

// failGraph: listItems (entry) -> getItem, plus a deliberate error: getItem with an unknown id (404 expected).
// The API answers status for the unknown id.
func failGraph(status int) []engine.Start {
	list := &engine.Action[noParams, page]{
		Name:     "listItems",
		Generate: func(*rand.Rand) noParams { return noParams{} },
		Do: func(context.Context, noParams, string) engine.Result[page] {
			return engine.Result[page]{Status: 200, Body: &page{IDs: []int{1, 2}}}
		},
	}
	get := &engine.Action[idParams, item]{
		Name: "getItem",
		Do: func(_ context.Context, p idParams, _ string) engine.Result[item] {
			return engine.Result[item]{Status: 200, Body: &item{p.ID}}
		},
	}
	unknown := engine.Breaker[idParams]{
		Kind: "unknown id", Expect: 404,
		Call: func(_ context.Context, p idParams, _ string) (int, any, error) {
			return status, idParams{ID: -p.ID}, nil
		},
	}
	pick := func(r *rand.Rand, _ noParams, out page) (idParams, bool) {
		return idParams{out.IDs[r.IntN(len(out.IDs))]}, true
	}
	list.Then(engine.Follow(get, pick))
	list.Then(engine.FailAll(get, []engine.Breaker[idParams]{unknown}, pick)...)
	get.Then(engine.Follow(list, func(*rand.Rand, idParams, item) (noParams, bool) { return noParams{}, true }))
	return []engine.Start{engine.StartWith(list, 1)}
}

func Test_Fail_NoErrorsWhenTheShareIsZero(t *testing.T) {
	// GIVEN users with no share of errors
	u := engine.NewUser("u", 1, engine.Config{Starts: failGraph(404), MaxSteps: 40, Picker: engine.Weighted{}})

	// WHEN they walk
	if err := u.Walk(context.Background()); err != nil {
		t.Fatal(err)
	}

	// THEN no step is a deliberate error
	for _, r := range u.Journal {
		if r.Intent != "" {
			t.Fatalf("deliberate error with a zero share: %+v", r)
		}
	}
}

func Test_Fail_ADeliberateErrorIsRecordedAndTheWalkGoesOnFromTheSamePlace(t *testing.T) {
	// GIVEN users that always pick a deliberate error when one is on offer
	all := func() float64 { return 1 }
	u := engine.NewUser("u", 1, engine.Config{Starts: failGraph(404), MaxSteps: 4, Picker: engine.Weighted{Errors: all}})

	// WHEN they walk
	if err := u.Walk(context.Background()); err != nil {
		t.Fatal(err)
	}

	// THEN after the list, every step is the deliberate error, recorded with its intent, from the same list
	if got := names(u); got != "listItems,getItem,getItem,getItem" {
		t.Fatalf("journal = %s", got)
	}
	for _, r := range u.Journal[1:] {
		if r.Intent != "unknown id" || r.Expect != 404 || r.Status != 404 || r.Missed() {
			t.Fatalf("record = %+v", r)
		}
		if p, ok := r.Params.(idParams); !ok || p.ID >= 0 {
			t.Fatalf("the journal must show what was sent: %+v", r.Params)
		}
	}
}

func Test_Fail_ADeliberateErrorThatIsNotRefusedIsMissed(t *testing.T) {
	// GIVEN an API that answers 200 where a 404 is expected
	all := func() float64 { return 1 }
	u := engine.NewUser("u", 1, engine.Config{Starts: failGraph(200), MaxSteps: 2, Picker: engine.Weighted{Errors: all}})

	// WHEN a user plays the deliberate error
	if err := u.Walk(context.Background()); err != nil {
		t.Fatal(err)
	}

	// THEN the record says it missed
	if r := u.Journal[1]; !r.Missed() {
		t.Fatalf("record = %+v", r)
	}
}

func Test_Fail_AScenarioPlaysOnlyNormalActions(t *testing.T) {
	// GIVEN a scenario on a graph with deliberate errors
	u := engine.NewUser("u", 1, engine.Config{Starts: failGraph(404), Picker: engine.Scripted{"listItems", "getItem", "listItems"}})

	// WHEN it is played
	if err := u.Walk(context.Background()); err != nil {
		t.Fatal(err)
	}

	// THEN no deliberate error was played
	for _, r := range u.Journal {
		if r.Intent != "" {
			t.Fatalf("deliberate error in a scenario: %+v", r)
		}
	}
}
