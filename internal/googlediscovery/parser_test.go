package googlediscovery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePreservesBasePathWithoutBaseURL(t *testing.T) {
	data := []byte(`{
		"kind":"discovery#restDescription",
		"name":"gmail",
		"rootUrl":"https://www.googleapis.com/",
		"basePath":"gmail/v1/",
		"resources":{"users":{"methods":{"getProfile":{
			"path":"users/{userId}/profile",
			"httpMethod":"GET",
			"parameters":{"userId":{"type":"string","location":"path","required":true}}
		}}}}
	}`)

	got, err := Parse("gmail.json", data)
	require.NoError(t, err)
	require.Equal(t, "https://www.googleapis.com/gmail/v1", got.BaseURL)
	require.Equal(t, "/users/{userId}/profile", got.Resources["users"].Endpoints["getProfile"].Path)
}

// A Google Discovery method with supportsMediaUpload: true still describes an
// ordinary JSON REST endpoint (path + httpMethod + request $ref); the media
// upload is a separately negotiated URL, not a replacement. Regression: the
// converter must not drop the JSON endpoint just because the method also
// advertises an upload variant.
func TestParsePreservesMediaCapableMethodJSONEndpoint(t *testing.T) {
	data := []byte(`{
		"kind":"discovery#restDescription",
		"name":"example",
		"baseUrl":"https://example.googleapis.com/v1/",
		"schemas":{"Item":{"type":"object","properties":{
			"id":{"type":"string"},
			"payload":{"type":"string"}
		}}},
		"resources":{"items":{"methods":{
			"create":{
				"path":"items",
				"httpMethod":"POST",
				"supportsMediaUpload":true,
				"request":{"$ref":"Item"},
				"response":{"$ref":"Item"}
			},
			"list":{
				"path":"items",
				"httpMethod":"GET"
			}
		}}}
	}`)

	got, err := Parse("example.json", data)
	require.NoError(t, err)

	items, ok := got.Resources["items"]
	require.True(t, ok, "items resource should be present")

	create, ok := items.Endpoints["create"]
	require.True(t, ok, "media-capable create endpoint should not be dropped")
	require.Equal(t, "POST", create.Method)
	require.Equal(t, "/items", create.Path)
	require.NotEmpty(t, create.Body, "request $ref should hydrate Body params for --stdin/body-field emission")

	bodyNames := make([]string, 0, len(create.Body))
	for _, p := range create.Body {
		bodyNames = append(bodyNames, p.Name)
	}
	require.ElementsMatch(t, []string{"id", "payload"}, bodyNames)

	_, hasList := items.Endpoints["list"]
	require.True(t, hasList, "sibling non-media method should still be present")
}

// Same invariant as TestParsePreservesMediaCapableMethodJSONEndpoint, but for
// the sub-resource path. Discovery nests resources, and the converter routes
// child methods through convertShallowResource — a distinct site that also
// used to drop media-capable methods. This locks the second site.
func TestParsePreservesMediaCapableMethodJSONEndpointInSubResource(t *testing.T) {
	data := []byte(`{
		"kind":"discovery#restDescription",
		"name":"example",
		"baseUrl":"https://example.googleapis.com/v1/",
		"schemas":{"Item":{"type":"object","properties":{
			"id":{"type":"string"},
			"payload":{"type":"string"}
		}}},
		"resources":{"widgets":{
			"resources":{"attachments":{"methods":{
				"upload":{
					"path":"widgets/{widgetId}/attachments/{attachmentId}",
					"httpMethod":"PUT",
					"supportsMediaUpload":true,
					"parameters":{
						"widgetId":{"type":"string","location":"path","required":true},
						"attachmentId":{"type":"string","location":"path","required":true}
					},
					"parameterOrder":["widgetId","attachmentId"],
					"request":{"$ref":"Item"}
				}
			}}}
		}}
	}`)

	got, err := Parse("example.json", data)
	require.NoError(t, err)

	widgets, ok := got.Resources["widgets"]
	require.True(t, ok, "parent resource of a media-capable sub-resource must be kept")
	attachments, ok := widgets.SubResources["attachments"]
	require.True(t, ok, "sub-resource holding only a media-capable method must be kept")

	upload, ok := attachments.Endpoints["upload"]
	require.True(t, ok, "media-capable sub-resource method must remain as JSON endpoint")
	require.Equal(t, "PUT", upload.Method)
	require.Equal(t, "/widgets/{widgetId}/attachments/{attachmentId}", upload.Path)
	require.NotEmpty(t, upload.Body, "request $ref must resolve to body params so --stdin and typed flags are emitted")
}

func TestParseUsesDiscoveryParameterOrderForPathArguments(t *testing.T) {
	data := []byte(`{
		"kind":"discovery#restDescription",
		"name":"example",
		"baseUrl":"https://example.googleapis.com/v1/",
		"resources":{"widgets":{"methods":{"get":{
			"path":"projects/{project}/widgets/{widget}",
			"httpMethod":"GET",
			"parameterOrder":["widget","project"],
			"parameters":{
				"project":{"type":"string","location":"path","required":true},
				"widget":{"type":"string","location":"path","required":true},
				"view":{"type":"string","location":"query"}
			}
		}}}}
	}`)

	got, err := Parse("example.json", data)
	require.NoError(t, err)
	params := got.Resources["widgets"].Endpoints["get"].Params
	require.Len(t, params, 3)
	require.Equal(t, []string{"widget", "project", "view"}, []string{params[0].Name, params[1].Name, params[2].Name})
}
