package collector

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type Cursor interface {
	Next(ctx context.Context) bool
	Close(ctx context.Context) error
	Decode(val interface{}) error
}

type ChangeStreamEventNamespace struct {
	DB   string
	Coll string
}

type ChangeStreamEvent struct {
	NS *ChangeStreamEventNamespace
}

type AggregationResult map[string]interface{}

type Driver interface {
	Connect(ctx context.Context, opts ...*options.ClientOptions) error
	Ping(ctx context.Context, rp *readpref.ReadPref) error
	Aggregate(ctx context.Context, db string, col string, pipeline bson.A) (Cursor, error)
	Watch(ctx context.Context, db string, col string, pipeline mongo.Pipeline) (*mongo.ChangeStream, error)
}

type MongoDBDriver struct {
	client *mongo.Client
}

func (mdb *MongoDBDriver) Connect(ctx context.Context, opts ...*options.ClientOptions) error {
	client, err := mongo.Connect(ctx, opts...)
	if err != nil {
		return err
	}

	mdb.client = client
	return nil
}

func (mdb *MongoDBDriver) Ping(ctx context.Context, rp *readpref.ReadPref) error {
	return mdb.client.Ping(ctx, rp)
}

func (mdb *MongoDBDriver) Aggregate(ctx context.Context, db string, col string, pipeline bson.A) (Cursor, error) {
	return mdb.client.Database(db).Collection(col).Aggregate(ctx, pipeline)
}

func (mdb *MongoDBDriver) Watch(ctx context.Context, db string, col string, pipeline mongo.Pipeline) (*mongo.ChangeStream, error) {
	return mdb.client.Database(db).Collection(col).Watch(ctx, pipeline)
}
