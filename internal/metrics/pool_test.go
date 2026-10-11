package metrics

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestPoolCollector reads two pools that have never connected, as serve
// has with an owner DSN: each one's ceiling and empty state, labelled by
// name, from the one collector the registry accepts.
func TestPoolCollector(t *testing.T) {
	newPool := func(maxConns string) *pgxpool.Pool {
		pool, err := pgxpool.New(t.Context(), "postgres://kritique@127.0.0.1:1/kritique?pool_max_conns="+maxConns)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
	app, owner := newPool("7"), newPool("2")
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPoolCollector(map[string]func() *pgxpool.Stat{"app": app.Stat, "owner": owner.Stat}))
	want := `# HELP kritique_db_pool_connections Connections the pool holds, by state: idle, acquired (in use), constructing.
# TYPE kritique_db_pool_connections gauge
kritique_db_pool_connections{pool="app",state="acquired"} 0
kritique_db_pool_connections{pool="app",state="constructing"} 0
kritique_db_pool_connections{pool="app",state="idle"} 0
kritique_db_pool_connections{pool="owner",state="acquired"} 0
kritique_db_pool_connections{pool="owner",state="constructing"} 0
kritique_db_pool_connections{pool="owner",state="idle"} 0
# HELP kritique_db_pool_max_connections The pool's ceiling on connections.
# TYPE kritique_db_pool_max_connections gauge
kritique_db_pool_max_connections{pool="app"} 7
kritique_db_pool_max_connections{pool="owner"} 2
# HELP kritique_db_pool_empty_acquires_total Connections taken from the pool after waiting for one to free: the pool was empty.
# TYPE kritique_db_pool_empty_acquires_total counter
kritique_db_pool_empty_acquires_total{pool="app"} 0
kritique_db_pool_empty_acquires_total{pool="owner"} 0
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want),
		"kritique_db_pool_connections", "kritique_db_pool_max_connections", "kritique_db_pool_empty_acquires_total"); err != nil {
		t.Fatal(err)
	}
	if n, err := testutil.GatherAndCount(reg); err != nil || n != 14 {
		t.Fatalf("gathered %d series, %v; want 14", n, err)
	}
}
