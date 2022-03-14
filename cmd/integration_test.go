package cmd

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ory/dockertest"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/tj/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var resource dockertest.Resource
var mongodbUri string
var client *mongo.Client

func TestMain(m *testing.M) {
	// uses a sensible default on windows (tcp/http) and linux/osx (socket)
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	// pulls an image, creates a container based on it and runs it
	opts := &dockertest.RunOptions{
		Entrypoint: []string{"/usr/bin/mongod", "--bind_ip_all", "--replSet", "rs0"},
		Repository: "mongo",
		Tag:        "4.2",
	}

	resource, err := pool.RunWithOptions(opts)
	if err != nil {
		log.Fatalf("Could not start resource: %s", err)
	}

	// exponential backoff-retry, because the application in the container might not be ready to accept connections yet
	if err := pool.Retry(func() error {
		var err error

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		//connect=direct is required since mongodb rs is not initialized, otherwise we would end up with a context timeout
		mongodbUri = fmt.Sprintf("mongodb://localhost:%s/?connect=direct", resource.GetPort("27017/tcp"))
		client, err = mongo.Connect(ctx, options.Client().ApplyURI(mongodbUri))
		if err != nil {
			return err
		}

		client.Database("admin").RunCommand(ctx, bson.M{"replSetInitiate": bson.D{}})
		time.Sleep(time.Second * 3)

		return client.Ping(context.TODO(), readpref.Primary())
	}); err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	code := m.Run()

	// You can't defer this because os.Exit doesn't care for defer
	if err := pool.Purge(resource); err != nil {
		log.Fatalf("Could not purge resource: %s", err)
	}

	os.Exit(code)
}

func setupTestData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := client.Database("mydb").Collection("objects").InsertOne(ctx, bson.M{
		"foo": "bar",
	})
	assert.NoError(t, err)

	client.Database("mydb").Collection("objects").InsertOne(ctx, bson.M{
		"foo": "foo",
	})
	client.Database("mydb").Collection("queue").InsertOne(ctx, bson.M{
		"class":  "foobar",
		"status": 1,
	})
	client.Database("mydb").Collection("queue").InsertOne(ctx, bson.M{
		"class":  "foobar",
		"status": 1,
	})
	client.Database("mydb").Collection("queue").InsertOne(ctx, bson.M{
		"class":  "bar",
		"status": 2,
	})
	client.Database("mydb").Collection("events").InsertOne(ctx, bson.M{
		"type":    "bar",
		"created": time.Now(),
	})
	client.Database("mydb").Collection("events").InsertOne(ctx, bson.M{
		"type":    "bar",
		"created": time.Now(),
	})
	client.Database("mydb").Collection("events").InsertOne(ctx, bson.M{
		"type":    "foo",
		"created": time.Now(),
	})
}

/*
Needs a http handler not registered at package level,
otherwise this fails because of multiple paths registered

func TestHealthz(t *testing.T) {
	args := []string{
		"-f", "../example/configv2.yaml",
		"-b", ":9413",
	}

	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)
	rootCmd.SetArgs(args)

	//binding is blocking, do this async but wait 200ms for tcp port to be open
	go rootCmd.Execute()
	time.Sleep(200 * time.Millisecond)

	resp, err := http.Get("http://localhost:9413/healthz")
	assert.NoError(t, err)

	var bytes []byte
	_, err = resp.Body.Read(bytes)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "OK", string(bytes))
}*/

func TestMetricsConfigv2(t *testing.T) {
	setupTestData(t)

	expected := map[string]string{
		"myapp_example_simplevalue_total":    `name:"myapp_example_simplevalue_total" help:"Simple gauge metric" type:GAUGE metric:<gauge:<value:2 > > `,
		"myapp_example_processes_total":      `name:"myapp_example_processes_total" help:"The total number of processes in a job queue" type:GAUGE metric:<label:<name:"status" value:"postponed" > label:<name:"type" value:"foobar" > gauge:<value:2 > > metric:<label:<name:"status" value:"processing" > label:<name:"type" value:"bar" > gauge:<value:1 > > `,
		"myapp_events_total":                 `name:"myapp_events_total" help:"The total number of events (created 1h ago or newer)" type:GAUGE metric:<label:<name:"type" value:"bar" > gauge:<value:2 > > metric:<label:<name:"type" value:"foo" > gauge:<value:1 > > `,
		"mongodb_query_exporter_query_total": `name:"mongodb_query_exporter_query_total" help:"How many MongoDB queries have been processed, partitioned by metric, server and status" type:COUNTER metric:<label:<name:"metric" value:"myapp_events_total" > label:<name:"result" value:"SUCCESS" > label:<name:"server" value:"main" > counter:<value:1 > > metric:<label:<name:"metric" value:"myapp_example_processes_total" > label:<name:"result" value:"SUCCESS" > label:<name:"server" value:"main" > counter:<value:1 > > metric:<label:<name:"metric" value:"myapp_example_simplevalue_total" > label:<name:"result" value:"SUCCESS" > label:<name:"server" value:"main" > counter:<value:1 > > `,
	}

	os.Setenv("MDBEXPORTER_SERVER_0_MONGODB_URI", mongodbUri)
	args := []string{
		"-f", "../example/config.yaml",
	}

	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)
	rootCmd.SetArgs(args)

	//binding is blocking, do this async but wait 200ms for tcp port to be open
	go rootCmd.Execute()
	time.Sleep(200 * time.Millisecond)

	resp, err := http.Get("http://localhost:9412/metrics")
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, 200)

	d := expfmt.NewDecoder(resp.Body, expfmt.ResponseFormat(resp.Header))
	found := 0

	for {
		fam := io_prometheus_client.MetricFamily{}
		if err = d.Decode(&fam); err != nil {
			break
		}

		//fmt.Printf("%s\n", fam.String())
		if val, ok := expected[*fam.Name]; ok {
			found++
			assert.Equal(t, val, fam.String())
		}
	}

	assert.Len(t, expected, found)
}
