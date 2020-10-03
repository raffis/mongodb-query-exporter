package v2

import (
	"os"
	"testing"

	"github.com/raffis/mongodb-query-exporter/x/zap"
)

func TestBuild(t *testing.T) {
	t.Run("Build collector", func(t *testing.T) {
		var conf = &Config{
			Log: zap.Config{
				Encoding: "console",
				Level:    "error",
			},
			Collectors: []*Collector{
				&Collector{
					MongoDB: &MongoDB{},
				},
			},
		}
		_, err := conf.Build()

		if err != nil {
			t.Error(err)
		}
	})

	t.Run("Changed bind adress is correct", func(t *testing.T) {
		var conf = &Config{
			Log: zap.Config{
				Encoding: "console",
				Level:    "error",
			},
			Bind: ":2222",
		}

		_, err := conf.Build()

		if err != nil {
			t.Error(err)
		}

		if conf.GetBindAddr() != ":2222" {
			t.Error("Expected bind address to be :2222")
		}
	})

	t.Run("MongoDB URI is overwriteable by env", func(t *testing.T) {
		var conf = &Config{
			Log: zap.Config{
				Encoding: "console",
				Level:    "error",
			},
			Collectors: []*Collector{
				&Collector{
					MongoDB: &MongoDB{
						URI: "mongodb://foo:27017",
					},
				},
				&Collector{
					MongoDB: &MongoDB{
						URI: "mongodb://foo2:27017",
					},
				},
			},
		}

		os.Setenv("MDBEXPORTER_COLLECTORS_0_MONGODB_URI", "mongodb://bar:27017")
		os.Setenv("MDBEXPORTER_COLLECTORS_1_MONGODB_URI", "mongodb://bar2:27017")
		_, err := conf.Build()
		if err != nil {
			t.Error(err)
		}

		if conf.Collectors[0].MongoDB.URI != "mongodb://bar:27017" {
			t.Errorf("Expected conf.Collectors[0].MongoDB.URI to be mongodb://bar:27017 but is %s", conf.Collectors[0].MongoDB.URI)
		}
		if conf.Collectors[1].MongoDB.URI != "mongodb://bar2:27017" {
			t.Errorf("Expected conf.Collectors[1].MongoDB.URI to be mongodb://bar2:27017 but is %s", conf.Collectors[1].MongoDB.URI)
		}
	})
}
