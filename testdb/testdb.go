package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type TestDatabase struct {
	t    *testing.T
	conn *pgx.Conn
	pool *pgxpool.Pool
}

func NewDatabase(t *testing.T) *TestDatabase {
	var db *pgx.Conn
	defaultDatabase := "postgres"
	password := os.Getenv("POSTGRES_PASSWORD")
	if password == "" {
		password = "postgres"
	}

	var port = 5432

	envVar, okPort := os.LookupEnv("TEST_POSTGRES_PORT")
	if okPort {
		p, err := strconv.ParseInt(envVar, 10, 32)
		require.NoError(t, err)
		port = int(p)
	}
	db = newConn(t, int(port), password, defaultDatabase)

	b := make([]byte, 8)
	_, err := rand.Read(b)
	require.NoError(t, err)

	databaseName := fmt.Sprintf("test_%s", hex.EncodeToString(b))

	_, err = db.Exec(t.Context(), fmt.Sprintf("CREATE DATABASE %s", databaseName))
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := db.Exec(context.Background(),
			fmt.Sprintf("DROP DATABASE %s", databaseName))
		require.NoError(t, err)
	})
	var subDB = newConn(t, port, password, databaseName)
	var subPool = newPool(t, port, password, databaseName)

	return &TestDatabase{
		t:    t,
		conn: subDB,
		pool: subPool,
	}
}

func (od *TestDatabase) Conn() *pgx.Conn {
	return od.conn
}

func (od *TestDatabase) Pool() *pgxpool.Pool {
	return od.pool
}

func (od *TestDatabase) ConnString() string {
	return od.conn.Config().ConnString()
}

func newConn(t *testing.T, port int, password, database string) *pgx.Conn {
	connStr := fmt.Sprintf("host=localhost port=%d user=postgres password=%s dbname=%s sslmode=disable",
		port, password, database)
	db, err := pgx.Connect(t.Context(), connStr)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := db.Close(context.Background())
		require.NoError(t, err)
	})
	return db
}

func newPool(t *testing.T, port int, password, database string) *pgxpool.Pool {
	connStr := fmt.Sprintf("host=localhost port=%d user=postgres password=%s dbname=%s sslmode=disable",
		port, password, database)
	pool, err := pgxpool.New(t.Context(), connStr)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()
	})
	return pool
}
