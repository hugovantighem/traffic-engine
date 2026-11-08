package engine_test

import (
	"context"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/hugovantighem/traffic-engine"
)

type item struct{ ID int }
type list struct{ Items []item }
type idParams struct{ ID int }
type noParams struct{}

// graph: list (entry) -> get -> {edit, list}; edit -> get. get fails (404) when failGet is set.
func graph(failGet bool, seen *[]string) []engine.Start {
	saw := func(name, traceparent string) { *seen = append(*seen, name+" "+traceparent) }
	listAll := &engine.Action[noParams, list]{
		Name:     "listItems",
		Generate: func(*rand.Rand) noParams { return noParams{} },
		Do: func(_ context.Context, _ noParams, tp string) engine.Result[list] {
			saw("listItems", tp)
			return engine.Result[list]{Status: 200, Body: &list{Items: []item{{1}, {2}, {3}}}}
		},
	}
	get := &engine.Action[idParams, item]{
		Name: "getItem",
		Do: func(_ context.Context, p idParams, tp string) engine.Result[item] {
			saw("getItem", tp)
			if failGet {
				return engine.Result[item]{Status: 404}
			}
			return engine.Result[item]{Status: 200, Body: &item{p.ID}}
		},
	}
	edit := &engine.Action[idParams, item]{
		Name: "editItem",
		Do: func(_ context.Context, p idParams, tp string) engine.Result[item] {
			saw("editItem", tp)
			return engine.Result[item]{Status: 200, Body: &item{p.ID}}
		},
	}
	listAll.Then(engine.Follow(get, func(r *rand.Rand, _ noParams, out list) (idParams, bool) {
		if len(out.Items) == 0 {
			return idParams{}, false
		}
		return idParams{ID: out.Items[r.IntN(len(out.Items))].ID}, true
	}))
	get.Then(
		engine.Follow(edit, func(_ *rand.Rand, _ idParams, out item) (idParams, bool) { return idParams(out), true }),
		engine.Follow(listAll, func(*rand.Rand, idParams, item) (noParams, bool) { return noParams{}, true }),
	)
	edit.Then(engine.Follow(get, func(_ *rand.Rand, in idParams, _ item) (idParams, bool) { return in, true }))
	return []engine.Start{engine.StartWith(listAll, 1)}
}

func names(u *engine.User) string {
	var n []string
	for _, r := range u.Journal {
		n = append(n, r.Action)
	}
	return strings.Join(n, ",")
}

func Test_Walk_FollowsTheDeclaredTransitions(t *testing.T) {
	var seen []string
	u := engine.NewUser("u", 1, engine.Config{Starts: graph(false, &seen), MaxSteps: 12, Picker: engine.Weighted{}})
	if err := u.Walk(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(u.Journal) != 12 || u.Journal[0].Action != "listItems" {
		t.Fatalf("journal = %s", names(u))
	}
	for i := 1; i < len(u.Journal); i++ {
		prev, cur := u.Journal[i-1].Action, u.Journal[i].Action
		allowed := map[string]string{"listItems": "getItem", "getItem": "editItem listItems", "editItem": "getItem"}
		if !strings.Contains(allowed[prev], cur) {
			t.Fatalf("%s cannot follow %s (journal: %s)", cur, prev, names(u))
		}
	}
}

func Test_Walk_SameSeedSameJourney(t *testing.T) {
	run := func(seed uint64) string {
		var seen []string
		u := engine.NewUser("u", seed, engine.Config{Starts: graph(false, &seen), MaxSteps: 15, Picker: engine.Weighted{StopProb: 0.1}})
		if err := u.Walk(context.Background()); err != nil {
			t.Fatal(err)
		}
		return names(u)
	}
	if a, b := run(7), run(7); a != b {
		t.Fatalf("same seed, different journeys:\n%s\n%s", a, b)
	}
	if run(7) == run(8) && run(7) == run(9) {
		t.Fatal("different seeds should not all give the same journey")
	}
}

func Test_Walk_StopsWhenThereIsNothingToFollow(t *testing.T) {
	var seen []string
	u := engine.NewUser("u", 1, engine.Config{Starts: graph(true, &seen), MaxSteps: 10, Picker: engine.Weighted{}})
	if err := u.Walk(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := names(u); got != "listItems,getItem" {
		t.Fatalf("journal = %s, want the walk to end after the 404", got)
	}
	if u.Journal[1].Status != 404 {
		t.Fatalf("status = %d", u.Journal[1].Status)
	}
}

func Test_Walk_ScriptedScenario(t *testing.T) {
	var seen []string
	u := engine.NewUser("u", 1, engine.Config{
		Starts: graph(false, &seen), MaxSteps: 10,
		Picker: engine.Scripted{"listItems", "getItem", "editItem", "getItem"},
	})
	if err := u.Walk(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := names(u); got != "listItems,getItem,editItem,getItem" {
		t.Fatalf("journal = %s", got)
	}
}

func Test_Walk_ScriptedStepNotReachable(t *testing.T) {
	var seen []string
	u := engine.NewUser("u", 1, engine.Config{Starts: graph(false, &seen), Picker: engine.Scripted{"listItems", "editItem"}})
	err := u.Walk(context.Background())
	if err == nil || !strings.Contains(err.Error(), `"editItem" is not reachable`) || !strings.Contains(err.Error(), "getItem") {
		t.Fatalf("err = %v", err)
	}
}

func Test_Walk_SendsAValidTraceparentAndRecordsItsTraceID(t *testing.T) {
	var seen []string
	u := engine.NewUser("u", 1, engine.Config{Starts: graph(false, &seen), Picker: engine.Scripted{"listItems"}})
	if err := u.Walk(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(u.Journal[0].TraceID) != 32 {
		t.Fatalf("trace id = %q", u.Journal[0].TraceID)
	}
	if !strings.HasPrefix(seen[0], "listItems 00-") {
		t.Fatalf("traceparent not sent: %v", seen)
	}
}

func Test_Weighted_HonoursWeights(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	c := []engine.Candidate{{Weight: 9}, {Weight: 1}}
	counts := [2]int{}
	for range 1000 {
		i, err := engine.Weighted{}.Pick(r, c, 0)
		if err != nil {
			t.Fatal(err)
		}
		counts[i]++
	}
	if counts[0] < 800 || counts[1] == 0 {
		t.Fatalf("counts = %v", counts)
	}
}
