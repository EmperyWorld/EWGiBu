package importpropose

import "testing"

func TestResolveComposeWorkspacePath(t *testing.T) {
	got, fb, err := resolveComposeWorkspacePath("docker/stack/stack4.yml", "../builders/app")
	if err != nil {
		t.Fatal(err)
	}
	if fb {
		t.Fatal("unexpected fallback")
	}
	if got != "docker/builders/app" {
		t.Fatalf("got %q", got)
	}
	got, fb, err = resolveComposeWorkspacePath("stack4.yml", "../builders/stack4-ocrmypdf")
	if err != nil {
		t.Fatal(err)
	}
	if !fb {
		t.Fatal("expected workspace-root fallback")
	}
	if got != "builders/stack4-ocrmypdf" {
		t.Fatalf("got %q", got)
	}
	got, fb, err = resolveComposeWorkspacePath("stack4.yml", "../logs/stack4/tika")
	if err != nil || !fb || got != "logs/stack4/tika" {
		t.Fatalf("logs rebase got=%q fb=%v err=%v", got, fb, err)
	}
	_, _, err = resolveComposeWorkspacePath("stack4.yml", "/etc/passwd")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
}
