package storage

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/jovil96/balanza-agent/internal/domain"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS readings (
		id         INTEGER PRIMARY KEY,
		weight     REAL,
		unit       TEXT,
		stable     INTEGER,
		raw        TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS counters (
		name       TEXT PRIMARY KEY,
		next_value INTEGER NOT NULL
	)`); err != nil {
		return nil, fmt.Errorf("create counters table: %w", err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO counters (name, next_value) VALUES ('ticket', 1)`); err != nil {
		return nil, fmt.Errorf("initialize ticket counter: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS products (
		name TEXT PRIMARY KEY COLLATE NOCASE
	)`); err != nil {
		return nil, fmt.Errorf("create products table: %w", err)
	}
	for _, product := range []string{"Cacao", "Arroz", "Café"} {
		if _, err := db.Exec(`INSERT OR IGNORE INTO products (name) VALUES (?)`, product); err != nil {
			return nil, fmt.Errorf("initialize products: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) ListProducts() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM products ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()
	var products []string
	for rows.Next() {
		var product string
		if err := rows.Scan(&product); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, product)
	}
	return products, rows.Err()
}

func (s *Store) AddProduct(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("el nombre del producto es obligatorio")
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO products (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("add product: %w", err)
	}
	return nil
}

func (s *Store) DeleteProduct(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("seleccione un producto")
	}
	if _, err := s.db.Exec(`DELETE FROM products WHERE name = ?`, name); err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	return nil
}

func (s *Store) PeekTicketNumber() (int64, error) {
	var number int64
	if err := s.db.QueryRow(`SELECT next_value FROM counters WHERE name = 'ticket'`).Scan(&number); err != nil {
		return 0, fmt.Errorf("read ticket counter: %w", err)
	}
	return number, nil
}

// NextTicketNumber reserves a unique persistent number. A reserved number is
// never reused, even if the physical printer later reports an error.
func (s *Store) NextTicketNumber() (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin ticket counter: %w", err)
	}
	defer tx.Rollback()
	var number int64
	if err := tx.QueryRow(`SELECT next_value FROM counters WHERE name = 'ticket'`).Scan(&number); err != nil {
		return 0, fmt.Errorf("read ticket counter: %w", err)
	}
	if _, err := tx.Exec(`UPDATE counters SET next_value = next_value + 1 WHERE name = 'ticket'`); err != nil {
		return 0, fmt.Errorf("increment ticket counter: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit ticket counter: %w", err)
	}
	return number, nil
}

func (s *Store) SaveReading(r domain.Reading) error {
	stable := 0
	if r.Stable {
		stable = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO readings (weight, unit, stable, raw) VALUES (?, ?, ?, ?)`,
		r.Weight, r.Unit, stable, r.Raw,
	)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
