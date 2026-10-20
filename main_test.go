package main

import "testing"

func TestHasHeadlessFlag(t *testing.T) {
	for _, args := range [][]string{{"--headless"}, {"-headless"}, {"/HEADLESS"}, {"otro", "--headless"}} {
		if !hasHeadlessFlag(args) {
			t.Fatalf("hasHeadlessFlag(%q) = false", args)
		}
	}
	if hasHeadlessFlag([]string{"--otro"}) {
		t.Fatal("una opción desconocida no debe activar el modo segundo plano")
	}
}
