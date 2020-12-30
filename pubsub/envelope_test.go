// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package pubsub

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCreateCloudEventsEnvelope(t *testing.T) {
	envelope := NewCloudEventsEnvelope("a", "source", "eventType", "", "", "", "", nil, "")
	assert.NotNil(t, envelope)
}

func TestEnvelopeXML(t *testing.T) {
	t.Run("xml content", func(t *testing.T) {
		str := `<root/>`
		envelope := NewCloudEventsEnvelope("a", "", "", "", "routed.topic", "mypubsub", "application/xml", []byte(str), "")
		assert.Equal(t, "application/xml", envelope.DataContentType)
		assert.Equal(t, str, envelope.Data)
		assert.Equal(t, "1.0", envelope.SpecVersion)
		assert.Equal(t, "routed.topic", envelope.Topic)
		assert.Equal(t, "mypubsub", envelope.PubsubName)
	})

	t.Run("xml without content-type", func(t *testing.T) {
		str := `<root/>`
		envelope := NewCloudEventsEnvelope("a", "", "", "", "routed.topic", "mypubsub", "", []byte(str), "")
		assert.Equal(t, "text/plain", envelope.DataContentType)
		assert.Equal(t, str, envelope.Data)
		assert.Equal(t, "1.0", envelope.SpecVersion)
		assert.Equal(t, "routed.topic", envelope.Topic)
		assert.Equal(t, "mypubsub", envelope.PubsubName)
	})
}

func TestCreateFromJSON(t *testing.T) {
	t.Run("has JSON object", func(t *testing.T) {
		obj1 := struct {
			Val1 string
			Val2 int
		}{
			"test",
			1,
		}
		data, _ := json.Marshal(obj1)
		envelope := NewCloudEventsEnvelope("a", "source", "", "", "", "mypubsub", "", data, "1")
		t.Logf("data: %v", envelope.Data)
		assert.Equal(t, "application/json", envelope.DataContentType)

		obj2 := struct {
			Val1 string
			Val2 int
		}{}
		err := json.Unmarshal(data, &obj2)
		assert.NoError(t, err)
		assert.Equal(t, obj1.Val1, obj2.Val1)
		assert.Equal(t, obj1.Val2, obj2.Val2)
	})
}

func TestCreateCloudEventsEnvelopeDefaults(t *testing.T) {
	t.Run("default event type", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "source", "", "", "", "mypubsub", "", nil, "")
		assert.Equal(t, DefaultCloudEventType, envelope.Type)
	})

	t.Run("non-default event type", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "source", "e1", "", "", "mypubsub", "", nil, "")
		assert.Equal(t, "e1", envelope.Type)
	})

	t.Run("spec version", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "source", "", "", "", "mypubsub", "", nil, "")
		assert.Equal(t, CloudEventsSpecVersion, envelope.SpecVersion)
	})

	t.Run("quoted data", func(t *testing.T) {
		list := []string{"v1", "v2", "v3"}
		data := strings.Join(list, ",")
		envelope := NewCloudEventsEnvelope("a", "source", "", "", "", "mypubsub", "", []byte(data), "")
		t.Logf("data: %v", envelope.Data)
		assert.Equal(t, "text/plain", envelope.DataContentType)
		assert.Equal(t, data, envelope.Data.(string))
	})

	t.Run("string data content type", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "source", "", "", "", "mypubsub", "", []byte("data"), "")
		assert.Equal(t, "text/plain", envelope.DataContentType)
	})

	t.Run("trace id", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "source", "", "", "", "mypubsub", "", []byte("data"), "1")
		assert.Equal(t, "1", envelope.DaprTraceID)
	})
}

func TestCreateCloudEventsEnvelopeExpiration(t *testing.T) {
	str := `{
		"specversion" : "1.0",
		"type" : "com.github.pull.create",
		"source" : "https://github.com/cloudevents/spec/pull",
		"subject" : "123",
		"id" : "A234-1234-1234",
		"comexampleextension1" : "value",
		"comexampleothervalue" : 5,
		"datacontenttype" : "text/xml",
		"data" : "<much wow=\"xml\"/>"
	}`

	t.Run("cloud event not expired", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "", "", "", "routed.topic", "mypubsub", "", []byte(str), "")
		envelope.Expiration = time.Now().UTC().Add(time.Hour * 24).Format(time.RFC3339)
		assert.False(t, envelope.HasExpired())
	})

	t.Run("cloud event expired", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "", "", "", "routed.topic", "mypubsub", "", []byte(str), "")
		envelope.Expiration = time.Now().UTC().Add(time.Hour * -24).Format(time.RFC3339)
		assert.True(t, envelope.HasExpired())
	})

	t.Run("cloud event expired but applied new TTL from metadata", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "", "", "", "routed.topic", "mypubsub", "", []byte(str), "")
		envelope.Expiration = time.Now().UTC().Add(time.Hour * -24).Format(time.RFC3339)
		envelope.ApplyMetadata(nil, map[string]string{
			"ttlInSeconds": "10000",
		})
		assert.NotEqual(t, "", envelope.Expiration)
		assert.False(t, envelope.HasExpired())
	})

	t.Run("cloud event TTL from metadata does not apply due to component feature", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "", "", "", "routed.topic", "mypubsub", "", []byte(str), "")
		envelope.ApplyMetadata([]Feature{FeatureMessageTTL}, map[string]string{
			"ttlInSeconds": "10000",
		})
		assert.Equal(t, "", envelope.Expiration)
		assert.False(t, envelope.HasExpired())
	})

	t.Run("cloud event with max TTL metadata", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "", "", "", "routed.topic", "mypubsub", "", []byte(str), "")
		envelope.ApplyMetadata(nil, map[string]string{
			"ttlInSeconds": fmt.Sprintf("%v", math.MaxInt64),
		})
		assert.NotEqual(t, "", envelope.Expiration)
		assert.False(t, envelope.HasExpired())
	})

	t.Run("cloud event with invalid expiration format", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "", "", "", "routed.topic", "mypubsub", "", []byte(str), "")
		envelope.Expiration = time.Now().UTC().Add(time.Hour * -24).Format(time.RFC1123)
		assert.False(t, envelope.HasExpired())
	})

	t.Run("cloud event without expiration", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "", "", "", "routed.topic", "mypubsub", "", []byte(str), "")
		assert.False(t, envelope.HasExpired())
	})

	t.Run("cloud event without expiration, without metadata", func(t *testing.T) {
		envelope := NewCloudEventsEnvelope("a", "", "", "", "routed.topic", "mypubsub", "", []byte(str), "")
		envelope.ApplyMetadata(nil, map[string]string{})
		assert.False(t, envelope.HasExpired())
	})
}

func TestSetTraceID(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		_, err := SetTraceID([]byte("a"), "1")
		assert.Error(t, err)
	})

	t.Run("valid json", func(t *testing.T) {
		m := map[string]interface{}{
			"specversion": "1.0",
			"customfield": "a",
		}

		b, err := json.Marshal(m)
		assert.NoError(t, err)
		ce, err := SetTraceID(b, "1")
		assert.NoError(t, err)

		json.Unmarshal(ce, &m)
		assert.Equal(t, "1.0", m["specversion"])
		assert.Equal(t, "1", m[DaprTraceIDField])
	})
}
