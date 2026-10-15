package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"iacpack/internal/canonicaljson"
	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	"iacpack/internal/domain/plan"
)

// Graph diagnostic codes.
const (
	CodeGraphCycle      = "GRAPH.CYCLE"
	CodeGraphInvalidDep = "GRAPH.INVALID_DEPENDENCY"
	CodeGraphInternal   = "GRAPH.INTERNAL"
)

// BuildGraph constructs the deterministic action DAG from IR.
func BuildGraph(d ir.Deployment) (plan.Graph, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	actions := map[string]plan.Action{}
	var edges []plan.Edge

	add := func(a plan.Action) {
		actions[a.ID] = a
	}
	edge := func(from, to, cond string) {
		if from == "" || to == "" || from == to {
			return
		}
		edges = append(edges, plan.Edge{From: from, To: to, Condition: cond})
	}

	add(plan.Action{ID: "action:verify", Kind: plan.KindVerify, Label: "verify"})
	add(plan.Action{ID: "action:finalize", Kind: plan.KindFinalize, Label: "finalize"})

	for _, id := range sortedKeys(d.Networks) {
		aid := "action:network/" + id
		add(plan.Action{ID: aid, Kind: plan.KindProvisionNetwork, Target: id, Label: "network " + id})
		edge("action:verify", aid, "")
	}
	for _, id := range sortedKeys(d.Volumes) {
		aid := "action:volume/" + id
		add(plan.Action{ID: aid, Kind: plan.KindProvisionVolume, Target: id, Label: "volume " + id})
		edge("action:verify", aid, "")
	}
	for _, id := range sortedKeys(d.Resources) {
		aid := "action:resource/" + id
		add(plan.Action{ID: aid, Kind: plan.KindProvisionResource, Target: id, Label: "resource " + id})
		edge("action:verify", aid, "")
	}
	for _, id := range sortedKeys(d.Images) {
		aid := "action:image/" + id
		add(plan.Action{ID: aid, Kind: plan.KindImageAvailable, Target: id, Label: "image " + id})
		edge("action:verify", aid, "")
	}

	svcStart := map[string]string{}
	svcReady := map[string]string{}
	for _, id := range sortedKeys(d.Services) {
		svc := d.Services[id]
		startID := "action:service/" + id + "/start"
		svcStart[id] = startID
		add(plan.Action{ID: startID, Kind: plan.KindServiceStart, Target: id, Label: "start " + id})
		edge("action:verify", startID, "")
		if _, ok := actions["action:image/"+svc.Image]; ok {
			edge("action:image/"+svc.Image, startID, "")
		}
		for _, n := range sortedCopy(svc.Networks) {
			edge("action:network/"+n, startID, "")
		}
		for _, m := range svc.Mounts {
			if m.Volume != "" {
				edge("action:volume/"+m.Volume, startID, "")
			}
			if m.Resource != "" {
				edge("action:resource/"+m.Resource, startID, "")
			}
		}
		if svc.Healthcheck != nil {
			readyID := "action:service/" + id + "/ready"
			svcReady[id] = readyID
			add(plan.Action{ID: readyID, Kind: plan.KindServiceReady, Target: id, Label: "ready " + id})
			edge(startID, readyID, "started")
		}
	}

	taskRun := map[string]string{}
	for _, id := range sortedKeys(d.Tasks) {
		t := d.Tasks[id]
		tid := "action:task/" + id
		taskRun[id] = tid
		add(plan.Action{ID: tid, Kind: plan.KindTaskRun, Target: id, Phase: t.Phase, Rerun: t.Rerun, Label: "task " + id})
		edge("action:verify", tid, "")
		if _, ok := actions["action:image/"+t.Image]; ok {
			edge("action:image/"+t.Image, tid, "")
		}
		for _, n := range sortedCopy(t.Networks) {
			edge("action:network/"+n, tid, "")
		}
		for _, m := range t.Mounts {
			if m.Volume != "" {
				if _, ok := actions["action:volume/"+m.Volume]; ok {
					edge("action:volume/"+m.Volume, tid, "")
				}
			}
			if m.Resource != "" {
				if _, ok := actions["action:resource/"+m.Resource]; ok {
					edge("action:resource/"+m.Resource, tid, "")
				}
			}
		}
	}

	// Authoring dependsOn edges (already phase-validated at decode time).
	for _, id := range sortedKeys(d.Services) {
		svc := d.Services[id]
		for _, dep := range svc.DependsOn {
			from, err := resolveEndpoint(dep, svcStart, svcReady, taskRun, d)
			if err != nil {
				diags = append(diags, diagnostic.Error(CodeGraphInvalidDep, err.Error()))
				continue
			}
			edge(from, svcStart[id], dep.Condition)
		}
	}
	for _, id := range sortedKeys(d.Tasks) {
		t := d.Tasks[id]
		for _, dep := range t.DependsOn {
			from, err := resolveEndpoint(dep, svcStart, svcReady, taskRun, d)
			if err != nil {
				diags = append(diags, diagnostic.Error(CodeGraphInvalidDep, err.Error()))
				continue
			}
			edge(from, taskRun[id], dep.Condition)
		}
	}

	// Finalize waits for all terminal container actions.
	for _, id := range sortedKeys(d.Services) {
		if ready, ok := svcReady[id]; ok {
			edge(ready, "action:finalize", "")
		} else {
			edge(svcStart[id], "action:finalize", "")
		}
	}
	for _, id := range sortedKeys(d.Tasks) {
		edge(taskRun[id], "action:finalize", "")
	}
	// Projects with only provision steps still finalize after verify.
	if len(d.Services) == 0 && len(d.Tasks) == 0 {
		edge("action:verify", "action:finalize", "")
	}

	edges = dedupeEdges(edges)
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Condition < edges[j].Condition
	})

	actList := make([]plan.Action, 0, len(actions))
	for _, id := range sortedKeys(actions) {
		actList = append(actList, actions[id])
	}

	if cycle := findCycle(actList, edges); len(cycle) > 0 {
		diags = append(diags, diagnostic.Error(CodeGraphCycle,
			fmt.Sprintf("dependency cycle: %s", strings.Join(cycle, " -> "))).
			WithHint("Break the cycle in dependsOn declarations"))
	}

	order, err := topoSort(actList, edges)
	if err != nil && !diagnostic.HasError(diags) {
		diags = append(diags, diagnostic.Error(CodeGraphInternal, err.Error()))
	}

	g := plan.Graph{
		Actions:        actList,
		Edges:          edges,
		Order:          order,
		DeploymentHash: d.SemanticHash,
	}
	// Graph identity hash over actions+edges (stable; excludes host metadata).
	if gh, err := graphContentHash(g); err == nil {
		_ = gh // deployment hash remains semantic; content hash available via inspect model
	}
	diagnostic.SortDeterministic(diags)
	return g, diags
}

func resolveEndpoint(dep ir.Dependency, svcStart, svcReady, taskRun map[string]string, d ir.Deployment) (string, error) {
	cond := dep.Condition
	if cond == "" {
		cond = "started"
	}
	if _, ok := d.Services[dep.Target]; ok {
		switch cond {
		case "healthy":
			ready, ok := svcReady[dep.Target]
			if !ok {
				return "", fmt.Errorf("healthy dependency on %q but service has no readiness action", dep.Target)
			}
			return ready, nil
		case "started":
			return svcStart[dep.Target], nil
		default:
			return "", fmt.Errorf("invalid condition %q for service target %q", cond, dep.Target)
		}
	}
	if _, ok := d.Tasks[dep.Target]; ok {
		switch cond {
		case "completedSuccessfully", "started":
			return taskRun[dep.Target], nil
		default:
			return "", fmt.Errorf("invalid condition %q for task target %q", cond, dep.Target)
		}
	}
	return "", fmt.Errorf("unknown dependency target %q", dep.Target)
}

func dedupeEdges(edges []plan.Edge) []plan.Edge {
	type key struct{ f, t, c string }
	seen := map[key]bool{}
	out := make([]plan.Edge, 0, len(edges))
	for _, e := range edges {
		k := key{e.From, e.To, e.Condition}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func findCycle(actions []plan.Action, edges []plan.Edge) []string {
	adj := map[string][]string{}
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	for id := range adj {
		sort.Strings(adj[id])
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	for _, a := range actions {
		color[a.ID] = white
	}
	var path []string
	var best []string
	var dfs func(u string) bool
	dfs = func(u string) bool {
		color[u] = gray
		path = append(path, u)
		for _, v := range adj[u] {
			if color[v] == gray {
				// cycle: from first occurrence of v in path
				i := indexOf(path, v)
				cyc := append(append([]string{}, path[i:]...), v)
				cyc = normalizeCycle(cyc)
				if len(best) == 0 || cycleLess(cyc, best) {
					best = cyc
				}
				path = path[:len(path)-1]
				color[u] = white
				return true
			}
			if color[v] == white {
				if dfs(v) {
					// continue searching for lexicographically smallest cycle
				}
			}
		}
		path = path[:len(path)-1]
		color[u] = black
		return len(best) > 0
	}
	ids := make([]string, 0, len(actions))
	for _, a := range actions {
		ids = append(ids, a.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if color[id] == white {
			_ = dfs(id)
		}
	}
	return best
}

func normalizeCycle(cyc []string) []string {
	if len(cyc) < 2 {
		return cyc
	}
	// cyc ends with repeat of start; body is cyc[:len-1]
	body := cyc[:len(cyc)-1]
	if len(body) == 0 {
		return cyc
	}
	minI := 0
	for i := 1; i < len(body); i++ {
		if body[i] < body[minI] {
			minI = i
		}
	}
	rot := append(append([]string{}, body[minI:]...), body[:minI]...)
	return append(rot, rot[0])
}

func cycleLess(a, b []string) bool {
	as := strings.Join(a, "\x00")
	bs := strings.Join(b, "\x00")
	return as < bs
}

func indexOf(ss []string, s string) int {
	for i, x := range ss {
		if x == s {
			return i
		}
	}
	return 0
}

func topoSort(actions []plan.Action, edges []plan.Edge) ([]string, error) {
	ids := make([]string, 0, len(actions))
	indeg := map[string]int{}
	adj := map[string][]string{}
	for _, a := range actions {
		ids = append(ids, a.ID)
		indeg[a.ID] = 0
	}
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
		indeg[e.To]++
	}
	for u := range adj {
		sort.Strings(adj[u])
	}
	sort.Strings(ids)
	var ready []string
	for _, id := range ids {
		if indeg[id] == 0 {
			ready = append(ready, id)
		}
	}
	var order []string
	for len(ready) > 0 {
		sort.Strings(ready)
		u := ready[0]
		ready = ready[1:]
		order = append(order, u)
		for _, v := range adj[u] {
			indeg[v]--
			if indeg[v] == 0 {
				ready = append(ready, v)
			}
		}
	}
	if len(order) != len(actions) {
		return order, fmt.Errorf("topological sort incomplete (cycle present)")
	}
	return order, nil
}

func graphContentHash(g plan.Graph) (string, error) {
	m := map[string]any{
		"actions": g.Actions,
		"edges":   g.Edges,
		"order":   g.Order,
	}
	raw, err := canonicaljson.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
