package postgres

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/newrelic/go-agent/v3/integrations/nrpgx5"
	"github.com/newrelic/go-agent/v3/newrelic"
)

func TestConnectionTracingUsesIndependentConcurrentTracers(t *testing.T) {
	config, err := pgxpool.ParseConfig("postgres://test:test@localhost:5432/tracing_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	configureConnectionTracing(config)
	if config.BeforeConnect == nil {
		t.Fatal("pool must install a per-connection tracing hook")
	}
	if config.ConnConfig.Tracer != nil {
		t.Fatal("pool template must not share a mutable connection tracer")
	}

	// pgxpool v5.9.2 copies ConnConfig before each concurrent BeforeConnect
	// invocation. Exercise that contract without opening a database connection.
	const count = 32
	type result struct {
		tracer *nrpgx5.Tracer
		err    error
	}
	results := make([]result, count)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := range results {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			connection := config.ConnConfig.Copy()
			connection.Host = fmt.Sprintf("database-%d.local", i)
			connection.Port = uint16(5400 + i)
			connection.Database = fmt.Sprintf("tenant_%d", i)
			<-start
			if err := config.BeforeConnect(context.Background(), connection); err != nil {
				results[i].err = err
				return
			}
			tracer, ok := connection.Tracer.(*nrpgx5.Tracer)
			if !ok {
				results[i].err = fmt.Errorf("connection tracer has type %T, want *nrpgx5.Tracer", connection.Tracer)
				return
			}
			results[i].tracer = tracer
			// The original shared tracer races here when MinConns starts more
			// than one connection, and can attach one database's metadata to another.
			for range 20 {
				ctx := tracer.TraceConnectStart(context.Background(), pgx.TraceConnectStartData{ConnConfig: connection})
				tracer.TraceConnectEnd(ctx, pgx.TraceConnectEndData{})
			}
		}(i)
	}
	close(start)
	workers.Wait()

	seen := make(map[*nrpgx5.Tracer]bool, count)
	for i, result := range results {
		if result.err != nil {
			t.Fatalf("connection %d: %v", i, result.err)
		}
		if seen[result.tracer] {
			t.Fatalf("connection %d reused another connection's tracer", i)
		}
		seen[result.tracer] = true
		if result.tracer.SendQueryParameters {
			t.Errorf("connection %d enables sensitive SQL query parameter telemetry", i)
		}
		segment := result.tracer.BaseSegment
		if segment.Host != fmt.Sprintf("database-%d.local", i) ||
			segment.PortPathOrID != strconv.Itoa(5400+i) ||
			segment.DatabaseName != fmt.Sprintf("tenant_%d", i) ||
			segment.Product != newrelic.DatastorePostgres {
			t.Errorf("connection %d lost its own datastore metadata: host=%q port=%q database=%q product=%q", i, segment.Host, segment.PortPathOrID, segment.DatabaseName, segment.Product)
		}
	}
	if config.ConnConfig.Tracer != nil || config.ConnConfig.Host != "localhost" || config.ConnConfig.Database != "tracing_test" {
		t.Fatal("per-connection tracing mutated the pool's shared configuration")
	}
}

func TestConnectionTracingPreservesQueryAndBatchInstrumentation(t *testing.T) {
	config, err := pgxpool.ParseConfig("postgres://test:test@localhost:5432/tracing_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	configureConnectionTracing(config)
	connection := config.ConnConfig.Copy()
	if err := config.BeforeConnect(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	tracer, ok := connection.Tracer.(*nrpgx5.Tracer)
	if !ok {
		t.Fatalf("connection tracer has type %T, want *nrpgx5.Tracer", connection.Tracer)
	}
	if _, ok := connection.Tracer.(pgx.ConnectTracer); !ok {
		t.Fatal("connection tracing is missing")
	}
	if _, ok := connection.Tracer.(pgx.PrepareTracer); !ok {
		t.Fatal("prepared statement instrumentation is missing")
	}
	queryTracer, ok := connection.Tracer.(pgx.QueryTracer)
	if !ok {
		t.Fatal("query instrumentation is missing")
	}
	batchTracer, ok := connection.Tracer.(pgx.BatchTracer)
	if !ok {
		t.Fatal("batch instrumentation is missing")
	}
	ctx := tracer.TraceConnectStart(context.Background(), pgx.TraceConnectStartData{ConnConfig: connection})
	tracer.TraceConnectEnd(ctx, pgx.TraceConnectEndData{})

	var querySegment *newrelic.DatastoreSegment
	parseQuery := tracer.ParseQuery
	if parseQuery == nil {
		t.Fatal("SQL parsing must remain enabled for datastore segment attribution")
	}
	tracer.ParseQuery = func(segment *newrelic.DatastoreSegment, query string) {
		parseQuery(segment, query)
		querySegment = segment
	}
	const query = "SELECT id FROM transactions WHERE id = $1"
	queryContext := queryTracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{
		SQL: query, Args: []any{"private-value-must-not-enter-telemetry"},
	})
	if queryContext == ctx || querySegment == nil {
		t.Fatal("query tracing did not create a datastore segment context")
	}
	if querySegment.Host != connection.Host || querySegment.DatabaseName != connection.Database ||
		querySegment.PortPathOrID != strconv.Itoa(int(connection.Port)) {
		t.Fatal("query segment did not inherit its connection's datastore metadata")
	}
	if querySegment.ParameterizedQuery != query || querySegment.Operation == "" || querySegment.Collection != "transactions" {
		t.Fatal("query segment lost SQL statement, operation, or collection attribution")
	}
	if len(querySegment.QueryParameters) != 0 || tracer.SendQueryParameters {
		t.Fatal("SQL arguments must not be copied into datastore telemetry")
	}
	queryTracer.TraceQueryEnd(queryContext, nil, pgx.TraceQueryEndData{})

	batch := &pgx.Batch{}
	batch.Queue(query, "private-value-must-not-enter-telemetry")
	batchContext := batchTracer.TraceBatchStart(ctx, nil, pgx.TraceBatchStartData{Batch: batch})
	if batchContext == ctx || batchContext == queryContext {
		t.Fatal("batch tracing did not create its own datastore segment context")
	}
	batchTracer.TraceBatchQuery(batchContext, nil, pgx.TraceBatchQueryData{SQL: query})
	batchTracer.TraceBatchEnd(batchContext, nil, pgx.TraceBatchEndData{})
	if tracer.BaseSegment.ParameterizedQuery != "" || tracer.BaseSegment.Operation != "" || len(tracer.BaseSegment.QueryParameters) != 0 {
		t.Fatal("query or batch tracing leaked per-operation data into the connection's base segment")
	}
}
