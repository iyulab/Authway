package config

import "testing"

func TestIsDevelopment(t *testing.T) {
	cases := map[string]bool{
		"":            true,
		"development": true,
		"staging":     false,
		"production":  false,
		"prod":        false, // unrecognized values fail closed, not open
	}
	for env, want := range cases {
		if got := IsDevelopment(env); got != want {
			t.Errorf("IsDevelopment(%q) = %v, want %v", env, got, want)
		}
	}
}

func TestIsProduction(t *testing.T) {
	cases := map[string]bool{
		"":            false,
		"development": false,
		"staging":     true,
		"production":  true,
		"prod":        true, // unrecognized values fail closed
	}
	for env, want := range cases {
		if got := IsProduction(env); got != want {
			t.Errorf("IsProduction(%q) = %v, want %v", env, got, want)
		}
	}
}
