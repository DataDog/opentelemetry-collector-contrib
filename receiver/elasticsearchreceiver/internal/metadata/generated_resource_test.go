// Code generated by mdatagen. DO NOT EDIT.

package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceBuilder(t *testing.T) {
	for _, test := range []string{"default", "all_set", "none_set"} {
		t.Run(test, func(t *testing.T) {
			cfg := loadResourceAttributesConfig(t, test)
			rb := NewResourceBuilder(cfg)
			rb.SetElasticsearchClusterName("elasticsearch.cluster.name-val")
			rb.SetElasticsearchIndexName("elasticsearch.index.name-val")
			rb.SetElasticsearchNodeName("elasticsearch.node.name-val")
			rb.SetElasticsearchNodeVersion("elasticsearch.node.version-val")

			res := rb.Emit()
			assert.Equal(t, 0, rb.Emit().Attributes().Len()) // Second call should return empty Resource

			switch test {
			case "default":
				assert.Equal(t, 4, res.Attributes().Len())
			case "all_set":
				assert.Equal(t, 4, res.Attributes().Len())
			case "none_set":
				assert.Equal(t, 0, res.Attributes().Len())
				return
			default:
				assert.Failf(t, "unexpected test case: %s", test)
			}

			val, ok := res.Attributes().Get("elasticsearch.cluster.name")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "elasticsearch.cluster.name-val", val.Str())
			}
			val, ok = res.Attributes().Get("elasticsearch.index.name")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "elasticsearch.index.name-val", val.Str())
			}
			val, ok = res.Attributes().Get("elasticsearch.node.name")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "elasticsearch.node.name-val", val.Str())
			}
			val, ok = res.Attributes().Get("elasticsearch.node.version")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "elasticsearch.node.version-val", val.Str())
			}
		})
	}
}
