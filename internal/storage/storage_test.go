package storage

import (
	"path/filepath"
	"testing"
)

func TestTicketCounterPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "counter.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.NextTicketNumber()
	if err != nil {
		t.Fatal(err)
	}
	if first != 1 {
		t.Fatalf("primer ticket = %d, esperado 1", first)
	}
	store.Close()

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	next, err := store.PeekTicketNumber()
	if err != nil {
		t.Fatal(err)
	}
	if next != 2 {
		t.Fatalf("siguiente ticket = %d, esperado 2", next)
	}
}

func TestProductCatalogPersistsAndDeletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "products.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddProduct("Quinua"); err != nil {
		t.Fatal(err)
	}
	store.Close()

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	products, err := store.ListProducts()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, product := range products {
		if product == "Quinua" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Quinua no persistió: %v", products)
	}
	if err := store.DeleteProduct("Quinua"); err != nil {
		t.Fatal(err)
	}
	products, err = store.ListProducts()
	if err != nil {
		t.Fatal(err)
	}
	for _, product := range products {
		if product == "Quinua" {
			t.Fatal("Quinua no fue eliminada")
		}
	}
}
