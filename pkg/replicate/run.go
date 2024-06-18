package replicate

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
	"gitlab.profetai.internal/profetai_ailm_team/online-deployment/libs/pg-replicate-sql/pkg/config"
	"gitlab.profetai.internal/profetai_ailm_team/online-deployment/libs/pg-replicate-sql/pkg/sqlgen"
	_ "modernc.org/sqlite"
)

func Run(ctx context.Context, cfg *config.Config) error {
	connStr := cfg.PostgresConnString() + "&replication=database"

	conn, err := replicateConnection(ctx, connStr, cfg.Replication.Publication)
	if err != nil {
		return fmt.Errorf("create replicate connection: %w", err)
	}
	defer conn.Close()

	// TODO: this is shared across reader and writer
	db, err := sqlx.Open("sqlite", cfg.Local.Path)
	if err != nil {
		return fmt.Errorf("connect to local db: %w", err)
	}

	sqliteCfg := sqlgen.SqliteConfig{
		SourceDB:    cfg.Upstream.DBName,
		Plugin:      cfg.Replication.Plugin,
		Publication: cfg.Replication.Publication,
	}

	driver := sqlgen.NewSqliteDriver(sqliteCfg, db)

	if err := driver.InitPositionTable(); err != nil {
		return fmt.Errorf("init position tracking: %w", err)
	}

	schema, err := driver.CurrentSchema()
	if err != nil {
		return fmt.Errorf("get current schema: %w", err)
	}

	sqlite := sqlgen.NewSqlite(sqliteCfg, schema)
	if err != nil {
		return fmt.Errorf("init sqlgen: %w", err)
	}

	slot := SlotConfig{
		SlotName:             cfg.Replication.SlotName,
		OutputPlugin:         cfg.Replication.Plugin,
		CreateSlotIfNoExists: cfg.Replication.CreateSlotIfNoExists,
		Temporary:            cfg.Replication.Temporary,
		Schema:               cfg.Upstream.Schema,
	}

	log.Debug().Msg("starting streaming")

	if err := conn.Stream(
		ctx,
		slot,
		driver,
		sqlite,
	); err != nil {
		return fmt.Errorf("streaming failed: %w", err)
	}

	return nil
}

func replicateConnection(ctx context.Context, connectionString, publication string) (*Conn, error) {
	conn, err := NewConn(ctx, connectionString, publication)
	if err != nil {
		return nil, fmt.Errorf("new conn: %w", err)
	}

	if err := conn.DropPublication(); err != nil {
		return nil, fmt.Errorf("drop publication: %w", err)
	}

	if err := conn.CreatePublication(); err != nil {
		return nil, fmt.Errorf("create publication: %w", err)
	}

	return conn, nil
}
