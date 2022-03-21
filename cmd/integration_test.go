package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/tj/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mongodbContainer struct {
	testcontainers.Container
	URI string
}

type integrationTest struct {
	name            string
	configPath      string
	mongodbImage    string
	expectedMetrics map[string]string
}

func TestMetricsConfigv2(t *testing.T) {
	expected := map[string]string{
		"myapp_example_simplevalue_total":    `name:"myapp_example_simplevalue_total" help:"Simple gauge metric" type:GAUGE metric:<gauge:<value:2 > > `,
		"myapp_example_processes_total":      `name:"myapp_example_processes_total" help:"The total number of processes in a job queue" type:GAUGE metric:<label:<name:"status" value:"postponed" > label:<name:"type" value:"foobar" > gauge:<value:2 > > metric:<label:<name:"status" value:"processing" > label:<name:"type" value:"bar" > gauge:<value:1 > > `,
		"myapp_events_total":                 `name:"myapp_events_total" help:"The total number of events (created 1h ago or newer)" type:GAUGE metric:<label:<name:"type" value:"bar" > gauge:<value:2 > > metric:<label:<name:"type" value:"foo" > gauge:<value:1 > > `,
		"mongodb_query_exporter_query_total": `name:"mongodb_query_exporter_query_total" help:"How many MongoDB queries have been processed, partitioned by metric, server and status" type:COUNTER metric:<label:<name:"aggregation" value:"aggregation_0" > label:<name:"result" value:"SUCCESS" > label:<name:"server" value:"main" > counter:<value:1 > > metric:<label:<name:"aggregation" value:"aggregation_1" > label:<name:"result" value:"SUCCESS" > label:<name:"server" value:"main" > counter:<value:1 > > metric:<label:<name:"aggregation" value:"aggregation_2" > label:<name:"result" value:"SUCCESS" > label:<name:"server" value:"main" > counter:<value:1 > > `,
	}

	tests := []integrationTest{
		/*integrationTest{
			name:            "integration test using config v1.0 and mongodb:5.0",
			configPath:      "../example/configv1.yaml",
			mongodbImage:    "mongo:5.0",
			expectedMetrics: expected,
		},*/
		integrationTest{
			name:            "integration test using config v2.0 and mongodb:5.0",
			configPath:      "../example/configv2.yaml",
			mongodbImage:    "mongo:5.0",
			expectedMetrics: expected,
		},
		integrationTest{
			name:            "integration test using config v3.0 and mongodb:4.4",
			configPath:      "../example/configv3.yaml",
			mongodbImage:    "mongo:4.4",
			expectedMetrics: expected,
		},
		integrationTest{
			name:            "integration test using config v3.0 and mongodb:5.0",
			configPath:      "../example/configv3.yaml",
			mongodbImage:    "mongo:5.0",
			expectedMetrics: expected,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executeIntegrationTest(t, test)
		})
	}
}

func executeIntegrationTest(t *testing.T, test integrationTest) {
	container, err := setupMongoDBContainer(context.TODO(), test.mongodbImage)
	assert.NoError(t, err)
	opts := options.Client().ApplyURI(container.URI)
	defer container.Terminate(context.TODO())

	client, err := mongo.Connect(context.TODO(), opts)
	assert.NoError(t, err)
	setupTestData(t, client)

	os.Setenv("MDBEXPORTER_SERVER_0_MONGODB_URI", container.URI)
	args := []string{
		"-f", test.configPath,
	}

	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)
	rootCmd.SetArgs(args)
	go rootCmd.Execute()

	//binding is blocking, do this async but wait 200ms for tcp port to be open
	time.Sleep(200 * time.Millisecond)
	resp, err := http.Get("http://localhost:9412/metrics")
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	d := expfmt.NewDecoder(resp.Body, expfmt.ResponseFormat(resp.Header))
	found := 0

	for {
		fam := io_prometheus_client.MetricFamily{}
		if err = d.Decode(&fam); err != nil {
			break
		}

		if val, ok := test.expectedMetrics[*fam.Name]; ok {
			found++
			assert.Equal(t, val, fam.String())
		}
	}

	assert.Len(t, test.expectedMetrics, found)

	//tear down http server and unregister collector
	srv.Shutdown(context.TODO())
	prometheus.Unregister(promCollector)
}

func setupMongoDBContainer(ctx context.Context, image string) (*mongodbContainer, error) {
	req := testcontainers.ContainerRequest{
		Image:        image,
		ExposedPorts: []string{"27017/tcp"},
		WaitingFor:   wait.ForListeningPort("27017"),
		Tmpfs: map[string]string{
			"/data/db": "",
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	if err != nil {
		return nil, err
	}

	ip, err := container.Host(ctx)
	if err != nil {
		return nil, err
	}

	mappedPort, err := container.MappedPort(ctx, "27017")
	if err != nil {
		return nil, err
	}

	uri := fmt.Sprintf("mongodb://%s:%s", ip, mappedPort.Port())

	return &mongodbContainer{Container: container, URI: uri}, nil
}

func setupTestData(t *testing.T, client *mongo.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
