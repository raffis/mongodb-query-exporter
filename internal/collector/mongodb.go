package collector

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Represents a cursor to fetch records from
type Cursor interface {
	Next(ctx context.Context) bool
	Close(ctx context.Context) error
	Decode(val interface{}) error
}

// MongoDB event stream
type ChangeStreamEventNamespace struct {
	DB   string
	Coll string
}

// MongoDB event stream
type ChangeStreamEvent struct {
	NS *ChangeStreamEventNamespace
}

// MongoDB aggregation result
type AggregationResult map[string]interface{}

// MongoDB driver abstraction
type Driver interface {
	Connect(ctx context.Context, opts ...*options.ClientOptions) error
	Ping(ctx context.Context, rp *readpref.ReadPref) error
	Aggregate(ctx context.Context, db string, col string, pipeline bson.A) (Cursor, error)
	Watch(ctx context.Context, db string, col string, pipeline bson.A) (Cursor, error)
}

// MongoDB driver
type MongoDBDriver struct {
	client *mongo.Client
}

// Connect to the server
func (mdb *MongoDBDriver) Connect(ctx context.Context, opts ...*options.ClientOptions) error {
	client, err := mongo.Connect(ctx, opts...)
	if err != nil {
		return err
	}

	mdb.client = client
	return nil
}

// Enforce connection to the server
func (mdb *MongoDBDriver) Ping(ctx context.Context, rp *readpref.ReadPref) error {
	return mdb.client.Ping(ctx, rp)
}

// Aggregation rquery
func (mdb *MongoDBDriver) Aggregate(ctx context.Context, db string, col string, pipeline bson.A) (Cursor, error) {
	return mdb.client.Database(db).Collection(col).Aggregate(ctx, pipeline)
}

// Start an eventstream
func (mdb *MongoDBDriver) Watch(ctx context.Context, db string, col string, pipeline bson.A) (Cursor, error) {
	return mdb.client.Database(db).Collection(col).Watch(ctx, pipeline)
}
