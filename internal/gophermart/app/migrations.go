package app

import (
	"embed"

	"fmt"

	"github.com/4aleksei/gmart/internal/common/logger"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

type gooseLogger struct {
	l *zap.Logger
}

func (l *gooseLogger) Fatalf(format string, v ...interface{}) {
	l.l.Fatal("goose fatal", zap.String("msg", fmt.Sprintf(format, v...)))
}
func (l *gooseLogger) Printf(format string, v ...interface{}) {
	l.l.Info("goose info", zap.String("msg", fmt.Sprintf(format, v...)))
}

func migrate(dbURI string, ll *logger.ZapLogger) error {
	var g = gooseLogger{l: ll.Logger}

	goose.SetLogger(&g)

	db, err := goose.OpenDBWithDriver("postgres", dbURI)
	if err != nil {
		return err
	}

	defer func() {
		db.Close()
	}()

	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	if err := goose.DownTo(db, "migrations", 0); err != nil {
		return err
	}

	if err := goose.Up(db, "migrations"); err != nil {
		return err
	}
	return nil
}
