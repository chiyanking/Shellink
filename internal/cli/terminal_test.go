package cli

import "testing"

func TestIsSessionReturnKey(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "escape", data: []byte{0x1b}, want: true},
		{name: "control right bracket", data: []byte{0x1d}, want: false},
		{name: "kitty control right bracket", data: []byte("\x1b[93;5u"), want: false},
		{name: "ordinary input", data: []byte("x"), want: false},
		{name: "arrow key", data: []byte("\x1b[A"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isSessionReturnKey(test.data); got != test.want {
				t.Fatalf("isSessionReturnKey(%q) = %t, want %t", test.data, got, test.want)
			}
		})
	}
}

func TestDashboardEscapeAction(t *testing.T) {
	tests := []struct {
		view     string
		nextView string
		exit     bool
	}{
		{view: "sessions", exit: true},
		{view: "profiles", nextView: "sessions"},
	}
	for _, test := range tests {
		t.Run(test.view, func(t *testing.T) {
			nextView, exit := dashboardEscapeAction(test.view)
			if nextView != test.nextView || exit != test.exit {
				t.Fatalf("dashboardEscapeAction(%q) = (%q, %t), want (%q, %t)", test.view, nextView, exit, test.nextView, test.exit)
			}
		})
	}
}
