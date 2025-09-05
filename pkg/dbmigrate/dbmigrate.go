// Package dbmigrate provides database migration functionality.
package dbmigrate

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var fs embed.FS

func Up(postgresURL string) error {
	d, err := iofs.New(fs, "")
	if err != nil {
		return fmt.Errorf("iofs.New: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, postgresURL)
	if err != nil {
		return fmt.Errorf("migrate.NewWithSourceInstance: %w", err)
	}
	err = m.Up()
	if err != nil {
		return fmt.Errorf("migrate.Up: %w", err)
	}
	return nil
}
