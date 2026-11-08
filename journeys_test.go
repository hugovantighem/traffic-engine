package engine_test

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	engine "github.com/hugovantighem/traffic-engine"
)

// journeyGraph: listItems (entry) -> getItem, listItems -> deleteItem in error only (404 expected),
// getItem -> listItems. deleteItem is in the graph but no normal journey reaches it.
func journeyGraph() *engine.Graph {
	list := &engine.Action[noParams, page]{
		Name:     "listItems",
		Generate: func(*rand.Rand) noParams { return noParams{} },
		Do: func(context.Context, noParams, string) engine.Result[page] {
			return engine.Result[page]{Status: 200, Body: &page{IDs: []int{1}}}
		},
	}
	get := &engine.Action[idParams, item]{Name: "getItem", Do: func(_ context.Context, p idParams, _ string) engine.Result[item] {
		return engine.Result[item]{Status: 200, Body: &item{p.ID}}
	}}
	del := &engine.Action[idParams, item]{Name: "deleteItem", Do: func(context.Context, idParams, string) engine.Result[item] {
		return engine.Result[item]{Status: 204}
	}}
	unknown := engine.Breaker[idParams]{Kind: "unknown id", Expect: 404, Call: func(context.Context, idParams, string) (int, any, error) {
		return 404, nil, nil
	}}
	pick := func(_ *rand.Rand, _ noParams, out page) (idParams, bool) { return idParams{out.IDs[0]}, true }
	list.Then(engine.Follow(get, pick), engine.Fail(del, unknown, pick))
	get.Then(engine.Follow(list, func(*rand.Rand, idParams, item) (noParams, bool) { return noParams{}, true }))
	return engine.NewGraph().Add(list, get, del).Start(engine.StartWith(list, 1))
}

func Test_Journeys_LeaveOutDeliberateErrors(t *testing.T) {
	// GIVEN a graph where deleteItem is only reached by a deliberate error
	g := journeyGraph()

	// WHEN its journeys are taken
	j := g.Journeys()

	// THEN only what a user can do is kept: no error link, no action reached only in error
	want := engine.Journeys{
		Starts: []string{"listItems"},
		Actions: []engine.Journey{
			{Name: "listItems", Next: []string{"getItem"}},
			{Name: "getItem", Next: []string{"listItems"}},
		},
	}
	if !reflect.DeepEqual(j, want) {
		t.Fatalf("journeys = %+v, want %+v", j, want)
	}
}

func Test_Journeys_WrittenAsJSON(t *testing.T) {
	// GIVEN a graph and a file in a directory that does not exist yet
	g := journeyGraph()
	path := filepath.Join(t.TempDir(), "web", "journeys.json")

	// WHEN the journeys are written
	err := g.WriteJourneys(path)

	// THEN the file holds the journeys, readable back
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var back engine.Journeys
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, g.Journeys()) {
		t.Fatalf("read back %+v, want %+v", back, g.Journeys())
	}
}
