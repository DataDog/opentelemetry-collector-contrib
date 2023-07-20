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
			rb.SetCloudProvider("cloud.provider-val")
			rb.SetHerokuAppID("heroku.app.id-val")
			rb.SetHerokuDynoID("heroku.dyno.id-val")
			rb.SetHerokuReleaseCommit("heroku.release.commit-val")
			rb.SetHerokuReleaseCreationTimestamp("heroku.release.creation_timestamp-val")
			rb.SetServiceInstanceID("service.instance.id-val")
			rb.SetServiceName("service.name-val")
			rb.SetServiceVersion("service.version-val")

			res := rb.Emit()
			assert.Equal(t, 0, rb.Emit().Attributes().Len()) // Second call should return 0

			switch test {
			case "default":
				assert.Equal(t, 8, res.Attributes().Len())
			case "all_set":
				assert.Equal(t, 8, res.Attributes().Len())
			case "none_set":
				assert.Equal(t, 0, res.Attributes().Len())
				return
			default:
				assert.Failf(t, "unexpected test case: %s", test)
			}

			val, ok := res.Attributes().Get("cloud.provider")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "cloud.provider-val", val.Str())
			}
			val, ok = res.Attributes().Get("heroku.app.id")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "heroku.app.id-val", val.Str())
			}
			val, ok = res.Attributes().Get("heroku.dyno.id")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "heroku.dyno.id-val", val.Str())
			}
			val, ok = res.Attributes().Get("heroku.release.commit")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "heroku.release.commit-val", val.Str())
			}
			val, ok = res.Attributes().Get("heroku.release.creation_timestamp")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "heroku.release.creation_timestamp-val", val.Str())
			}
			val, ok = res.Attributes().Get("service.instance.id")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "service.instance.id-val", val.Str())
			}
			val, ok = res.Attributes().Get("service.name")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "service.name-val", val.Str())
			}
			val, ok = res.Attributes().Get("service.version")
			assert.True(t, ok)
			if ok {
				assert.EqualValues(t, "service.version-val", val.Str())
			}
		})
	}
}
