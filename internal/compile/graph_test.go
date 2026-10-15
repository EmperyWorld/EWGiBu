package compile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/domain/plan"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
)

func TestReleaseExampleGraphStable(t *testing.T) {
	p := loadProject(t, "examples/release/iacpack.yaml")
	d1, err := compile.ToIR(p)
	if err != nil {
		t.Fatal(err)
	}
	g1, diags := compile.BuildGraph(d1)
	if diagnostic.HasError(diags) {
		t.Fatal(diags)
	}
	d2, _ := compile.ToIR(p)
	g2, _ := compile.BuildGraph(d2)
	if compile.FormatGraphText(d1, g1) != compile.FormatGraphText(d2, g2) {
		t.Fatal("text unstable")
	}
	if compile.FormatGraphMermaid(g1) != compile.FormatGraphMermaid(g2) {
		t.Fatal("mermaid unstable")
	}
	if g1.DeploymentHash != d1.SemanticHash {
		t.Fatal("deployment hash mismatch")
	}
	assertEdge(t, g1, "action:service/db/ready", "action:task/migrate", "healthy")
	assertEdge(t, g1, "action:service/db/ready", "action:service/app/start", "healthy")
	assertOrderBefore(t, g1, "action:image/app", "action:service/app/start")
	assertOrderBefore(t, g1, "action:verify", "action:finalize")
}

func TestXUIDoesNotChangeGraphHash(t *testing.T) {
	p := loadProject(t, "examples/release/iacpack.yaml")
	d1, _ := compile.ToIR(p)
	g1, _ := compile.BuildGraph(d1)
	n := p.Spec.XUI.Nodes["app"]
	n.X = 999
	p.Spec.XUI.Nodes["app"] = n
	d2, _ := compile.ToIR(p)
	g2, _ := compile.BuildGraph(d2)
	if g1.DeploymentHash != g2.DeploymentHash {
		t.Fatal("x-ui changed deployment hash")
	}
	m1 := compile.GraphInspectModel(d1, g1)
	m2 := compile.GraphInspectModel(d2, g2)
	if m1["graphHash"] != m2["graphHash"] {
		t.Fatal("x-ui changed graphHash")
	}
}

func TestSemanticChangeAltersHash(t *testing.T) {
	p := loadProject(t, "examples/release/iacpack.yaml")
	d1, _ := compile.ToIR(p)
	svc := p.Spec.Services["app"]
	svc.Command = []domain.ArgvElement{{Literal: "/app/server2"}}
	p.Spec.Services["app"] = svc
	d2, _ := compile.ToIR(p)
	if d1.SemanticHash == d2.SemanticHash {
		t.Fatal("expected semantic hash change")
	}
}

func TestCycleTwoServices(t *testing.T) {
	d := baseIR()
	d.Services = map[string]ir.Service{
		"a": {Image: "app", DependsOn: []ir.Dependency{{Target: "b", Condition: "started"}}},
		"b": {Image: "app", DependsOn: []ir.Dependency{{Target: "a", Condition: "started"}}},
	}
	_, diags := compile.BuildGraph(d)
	found := false
	for _, diag := range diags {
		if diag.Code == compile.CodeGraphCycle {
			found = true
			if !strings.Contains(diag.Message, "action:service/") {
				t.Fatalf("cycle path=%s", diag.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected cycle, got %v", diags)
	}
}

func TestCycleTasks(t *testing.T) {
	d := baseIR()
	d.Services = map[string]ir.Service{}
	d.Tasks = map[string]ir.Task{
		"t1": {Phase: "preStart", Image: "app", Command: ir.LiteralArgs("x"), DependsOn: []ir.Dependency{{Target: "t2", Condition: "completedSuccessfully"}}},
		"t2": {Phase: "preStart", Image: "app", Command: ir.LiteralArgs("y"), DependsOn: []ir.Dependency{{Target: "t1", Condition: "completedSuccessfully"}}},
	}
	_, diags := compile.BuildGraph(d)
	if !diagnostic.HasError(diags) {
		t.Fatal("expected cycle")
	}
}

func TestMermaidEscapesInjection(t *testing.T) {
	g := plan.Graph{
		Actions: []plan.Action{{ID: "action:verify", Kind: plan.KindVerify, Label: `evil"] ; flowchart TD; x["`}},
		Order:   []string{"action:verify"},
	}
	out := compile.FormatGraphMermaid(g)
	if strings.Contains(out, `evil"]`) {
		t.Fatalf("unescaped label: %s", out)
	}
	if !strings.Contains(out, `#quot;`) {
		t.Fatalf("expected escaped quote: %s", out)
	}
}

func TestTopoRespectsEdges(t *testing.T) {
	p := loadProject(t, "examples/release/iacpack.yaml")
	d, _ := compile.ToIR(p)
	g, diags := compile.BuildGraph(d)
	if diagnostic.HasError(diags) {
		t.Fatal(diags)
	}
	pos := map[string]int{}
	for i, id := range g.Order {
		pos[id] = i
	}
	for _, e := range g.Edges {
		if pos[e.From] >= pos[e.To] {
			t.Fatalf("edge violated: %s -> %s", e.From, e.To)
		}
	}
}

func TestLargeDAG500(t *testing.T) {
	services := map[string]ir.Service{}
	for i := 0; i < 500; i++ {
		id := "s" + pad3(i)
		svc := ir.Service{Image: "app"}
		if i > 0 {
			svc.DependsOn = []ir.Dependency{{Target: "s" + pad3(i-1), Condition: "started"}}
		}
		services[id] = svc
	}
	d := baseIR()
	d.Services = services
	g, diags := compile.BuildGraph(d)
	if diagnostic.HasError(diags) {
		t.Fatal(diags)
	}
	if len(g.Actions) < 500 {
		t.Fatalf("actions=%d", len(g.Actions))
	}
}

func TestDiamondAndDisconnected(t *testing.T) {
	d := baseIR()
	d.Services = map[string]ir.Service{
		"root":  {Image: "app"},
		"left":  {Image: "app", DependsOn: []ir.Dependency{{Target: "root", Condition: "started"}}},
		"right": {Image: "app", DependsOn: []ir.Dependency{{Target: "root", Condition: "started"}}},
		"join": {Image: "app", DependsOn: []ir.Dependency{
			{Target: "left", Condition: "started"},
			{Target: "right", Condition: "started"},
		}},
		"lonely": {Image: "app"},
	}
	g, diags := compile.BuildGraph(d)
	if diagnostic.HasError(diags) {
		t.Fatal(diags)
	}
	assertOrderBefore(t, g, "action:service/root/start", "action:service/left/start")
	assertOrderBefore(t, g, "action:service/left/start", "action:service/join/start")
	assertOrderBefore(t, g, "action:service/right/start", "action:service/join/start")
}

func baseIR() ir.Deployment {
	return ir.Deployment{
		FormatVersion: ir.FormatVersion,
		Project:       ir.ProjectRef{Name: "t", Version: "0.1.0"},
		Mode:          "release",
		Profile:       "compose-docker/v1",
		SemanticHash:  strings.Repeat("a", 64),
		Images:        map[string]ir.Image{"app": {Ref: "x@sha256:" + strings.Repeat("1", 64), Delivery: "pull"}},
		Parameters:    map[string]ir.Parameter{},
		Secrets:       map[string]ir.Secret{},
		Resources:     map[string]ir.Resource{},
		Networks:      map[string]ir.Network{},
		Volumes:       map[string]ir.Volume{},
		Services:      map[string]ir.Service{},
		Tasks:         map[string]ir.Task{},
	}
}

func loadProject(t *testing.T, rel string) domain.Project {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	if err != nil {
		t.Fatal(err)
	}
	doc := loader.Parse(filepath.Base(rel), data)
	p, diags := v1alpha1.Decode(doc)
	if diagnostic.HasError(diags) {
		t.Fatalf("decode: %v", diags)
	}
	return p
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func assertEdge(t *testing.T, g plan.Graph, from, to, cond string) {
	t.Helper()
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Condition == cond {
			return
		}
	}
	t.Fatalf("missing edge %s -> %s [%s]", from, to, cond)
}

func assertOrderBefore(t *testing.T, g plan.Graph, earlier, later string) {
	t.Helper()
	pos := map[string]int{}
	for i, id := range g.Order {
		pos[id] = i
	}
	if pos[earlier] >= pos[later] {
		t.Fatalf("want %s before %s", earlier, later)
	}
}

func pad3(i int) string {
	s := itoa(i)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
