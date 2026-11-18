package gemmalive

import "testing"

func TestSkipUnlessOptedIn(t *testing.T) {
	t.Setenv(Env, "")
	if Enabled() {
		t.Fatal("live Gemma tests enabled without opt-in")
	}
	t.Setenv(Env, "1")
	if !Enabled() {
		t.Fatal("opt-in not honored")
	}
}
