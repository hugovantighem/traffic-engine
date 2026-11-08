package engine

// Linked is an action as a Graph sees it: a name and its links (*Action is one).
type Linked interface {
	ActionName() string
	Links() []Link
}

// Graph is what a system plays: the actions it keeps, from any number of packages (one per
// service, each publishing its actions and their own links), and where walks start.
//
// A system links the actions of two services with Then, from its own package, without
// redefining them:
//
//	purchases.Record.Then(engine.Follow(points.Balance, func(_ *rand.Rand, _ purchases.RecordP, out Purchase) (points.BalanceP, bool) {
//		return points.BalanceP{CustomerID: out.CustomerID}, true
//	}))
//
// An action that is not added to the graph is never played, even when another action links to
// it: a system leaves out what only another service calls.
type Graph struct {
	actions []Linked
	known   map[string]bool
	starts  []Start
}

// NewGraph returns an empty graph.
func NewGraph() *Graph { return &Graph{known: map[string]bool{}} }

// Add keeps actions in the graph.
func (g *Graph) Add(actions ...Linked) *Graph {
	for _, a := range actions {
		if !g.known[a.ActionName()] {
			g.known[a.ActionName()] = true
			g.actions = append(g.actions, a)
		}
	}
	return g
}

// Start adds entry points of the walks.
func (g *Graph) Start(s ...Start) *Graph {
	g.starts = append(g.starts, s...)
	return g
}

// Has tells whether an action is in the graph.
func (g *Graph) Has(action string) bool { return g.known[action] }

// Starts are the entry points whose action is in the graph.
func (g *Graph) Starts() []Start {
	var out []Start
	for _, s := range g.starts {
		if g.known[s.Action] {
			out = append(out, s)
		}
	}
	return out
}

// Nodes are the actions of the graph with their links to actions of the graph, as they are now
// (links added after Add count): what a run should cover.
func (g *Graph) Nodes() []Node {
	out := make([]Node, 0, len(g.actions))
	for _, a := range g.actions {
		n := Node{Name: a.ActionName()}
		for _, l := range a.Links() {
			if g.known[l.To] {
				n.Links = append(n.Links, l)
			}
		}
		out = append(out, n)
	}
	return out
}

// Config completes a user configuration with the graph: its entry points, and only its actions.
func (g *Graph) Config(c Config) Config {
	c.Starts, c.Only = g.Starts(), g.Has
	return c
}

// Cover compares journals with what the graph declares (see Cover).
func (g *Graph) Cover(journals ...[]Record) Covered { return Cover(g.Starts(), g.Nodes(), journals...) }
