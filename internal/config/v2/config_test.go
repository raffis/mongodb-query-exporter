package v2

import (
	"os"
	"testing"

	"github.com/raffis/mongodb-query-exporter/v5/internal/x/zap"
	"github.com/tj/assert"
)

func TestBuild(t *testing.T) {
	t.Run("Build collector", func(t *testing.T) {
		var conf = &Config{}
		_, err := conf.Build()

		assert.NoError(t, err)
	})

	t.Run("Changed bind address is correct", func(t *testing.T) {
		var conf = &Config{
			Log: zap.Config{
				Encoding: "console",
				Level:    "error",
			},
			Bind: ":2222",
		}

		_, err := conf.Build()

		assert.NoError(t, err)
		assert.Equal(t, conf.GetBindAddr(), ":2222", "Expected bind address to be equal")
	})

	t.Run("Server is registered with name taken from mongodb URI", func(t *testing.T) {
		var conf = &Config{
			Log: zap.Config{
				Encoding: "console",
				Level:    "error",
			},
			Servers: []*Server{
				{
					URI: "mongodb://foo:27017,bar:27017",
				},
			},
		}

		c, err := conf.Build()

		assert.NoError(t, err)
		assert.Len(t, c.GetServers([]string{"foo:27017,bar:27017"}), 1, "Expected to found one server named foo:27017,bar:27017")
	})

	t.Run("Default server main localhost:27017 is applied if no servers are configured", func(t *testing.T) {
		var conf = &Config{
			Log: zap.Config{
				Encoding: "console",
				Level:    "error",
			},
		}
		c, err := conf.Build()
		assert.NoError(t, err)
		assert.Len(t, c.GetServers([]string{"main"}), 1, "Expected to found one server named main")
	})

	t.Run("Server name is changeable", func(t *testing.T) {
		var conf = &Config{
			Log: zap.Config{
				Encoding: "console",
				Level:    "error",
			},
			Servers: []*Server{
				{
					Name: "foo",
					URI:  "mongodb://foo:27017",
				},
			},
		}

		c, err := conf.Build()

		assert.NoError(t, err)
		assert.Len(t, c.GetServers([]string{"foo"}), 1, "Expected to found one server named foo")
	})

	t.Run("MongoDB URI is overwriteable by env", func(t *testing.T) {
		var conf = &Config{
			Log: zap.Config{
				Encoding: "console",
				Level:    "error",
			},
			Servers: []*Server{
				{
					Name: "foo",
					URI:  "mongodb://foo:27017",
				},
				{
					Name: "foo2",
					URI:  "mongodb://foo2:27017",
				},
			},
		}

		os.Setenv("MDBEXPORTER_SERVER_0_MONGODB_URI", "mongodb://bar:27017")
		os.Setenv("MDBEXPORTER_SERVER_1_MONGODB_URI", "mongodb://bar2:27017")
		_, err := conf.Build()
		assert.NoError(t, err)
		assert.Equal(t, conf.Servers[0].URI, "mongodb://bar:27017", "Expected conf.Collectors[0].MongoDB.URI to be mongodb://bar:27017")
		assert.Equal(t, conf.Servers[1].URI, "mongodb://bar2:27017", "Expected conf.Collectors[0].MongoDB.URI to be mongodb://bar2:27017")
	})
}
