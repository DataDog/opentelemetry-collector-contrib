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
			rb.SetOpencensusResourcetype("opencensus.resourcetype-val")
			rb.SetOpenshiftClusterquotaName("openshift.clusterquota.name-val")
			rb.SetOpenshiftClusterquotaUID("openshift.clusterquota.uid-val")

			res := rb.Emit()
			assert.Equal(t, 0, rb.Emit().Attributes().Len()) // Second call should return 0

			switch test {
			case "default":
				assert.Equal(t, 3, res.Attributes().Len())
			case "all_set":
				assert.Equal(t, 3, res.Attributes().Len())
			case "none_set":
				assert.Equal(t, 0, res.Attributes().Len())
				return
			default:
				assert.Failf(t, "unexpected test case: %s", test)
			}

			val, ok := res.Attributes().Get("opencensus.resourcetype")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "opencensus.resourcetype-val", val.Str())
			}
			val, ok = res.Attributes().Get("openshift.clusterquota.name")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "openshift.clusterquota.name-val", val.Str())
			}
			val, ok = res.Attributes().Get("openshift.clusterquota.uid")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "openshift.clusterquota.uid-val", val.Str())
			}
		})
	}
}
