package v1

import (
	"os"
	"testing"
)

func TestBuild(t *testing.T) {
	t.Run("Build collector", func(t *testing.T) {
		var conf = &Config{
			LogLevel: "error",
			MongoDB:  MongoDB{},
		}
		_, err := conf.Build()

		if err != nil {
			t.Error(err)
		}
	})

	t.Run("Changed bind address is correct", func(t *testing.T) {
		var conf = &Config{
			LogLevel: "error",
			Bind:     ":2222",
			MongoDB:  MongoDB{},
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
			LogLevel: "error",
			Bind:     ":2222",
			MongoDB: MongoDB{
				URI: "mongodb://foo:27017",
			},
		}

		os.Setenv("MDBEXPORTER_SERVER_0_MONGODB_URI", "mongodb://bar:27017")
		_, err := conf.Build()
		if err != nil {
			t.Error(err)
		}

		if conf.MongoDB.URI != "mongodb://bar:27017" {
			t.Errorf("Expected conf.Collectors[0].MongoDB.URI to be mongodb://bar:27017 but is %s", conf.MongoDB.URI)
		}
	})
}
