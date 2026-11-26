package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

// The OpenAPI document is generated from the same route table that registers the v1 handlers
// (v1.go) and from the v1 Go types, so the published description cannot drift from what the
// server does. Field descriptions come from `doc:"..."` struct tags, allowed values from
// `enum:"a,b"`. docs/openapi.json is a committed copy, kept current by a test.

const openAPIVersion = "3.1.0"

// apiDocVersion versions the API description itself: the minor goes up with each addition to v1.
const apiDocVersion = "1.0.0"

// DefaultServerURL is the server named in the committed docs/openapi.json.
const DefaultServerURL = "http://127.0.0.1:8480"

// OpenAPIJSON returns the OpenAPI document, indented, for docs/openapi.json and
// `rigctlweb openapi`. programVersion is recorded when non-empty.
func OpenAPIJSON(programVersion, serverURL string) []byte {
	b, _ := json.MarshalIndent(openAPI(programVersion, serverURL), "", "  ")
	return append(b, '\n')
}

type schemaSet struct {
	defs map[string]any
}

var timeType = reflect.TypeOf(time.Time{})

// ref returns the schema for t, registering named struct types under components/schemas.
func (s *schemaSet) ref(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == timeType:
		return map[string]any{"type": "string", "format": "date-time"}
	case t.Kind() == reflect.Struct:
		name := schemaName(t)
		if _, done := s.defs[name]; !done {
			s.defs[name] = nil // reserve: stops recursion
			s.defs[name] = s.object(t)
		}
		return map[string]any{"$ref": "#/components/schemas/" + name}
	case t.Kind() == reflect.Slice:
		return map[string]any{"type": "array", "items": s.ref(t.Elem())}
	case t.Kind() == reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": s.ref(t.Elem())}
	case t.Kind() == reflect.String:
		return map[string]any{"type": "string"}
	case t.Kind() == reflect.Bool:
		return map[string]any{"type": "boolean"}
	case t.Kind() >= reflect.Int && t.Kind() <= reflect.Uint64:
		f := "int64"
		if t.Kind() == reflect.Int32 || t.Kind() == reflect.Uint32 {
			f = "int32"
		}
		return map[string]any{"type": "integer", "format": f}
	case t.Kind() == reflect.Float32 || t.Kind() == reflect.Float64:
		return map[string]any{"type": "number"}
	}
	return map[string]any{}
}

func (s *schemaSet) object(t reflect.Type) map[string]any {
	props := map[string]any{}
	var order, required []string
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		p := s.ref(f.Type)
		if d := f.Tag.Get("doc"); d != "" {
			if _, isRef := p["$ref"]; isRef {
				// 3.1 allows siblings next to $ref.
				p = map[string]any{"$ref": p["$ref"], "description": d}
			} else {
				p["description"] = d
			}
		}
		if e := f.Tag.Get("enum"); e != "" {
			p["enum"] = strings.Split(e, ",")
		}
		props[name] = p
		order = append(order, name)
		if !strings.Contains(opts, "omitempty") {
			required = append(required, name)
		}
	}
	o := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		o["required"] = required
	}
	if d, ok := typeDocs[t]; ok {
		o["description"] = d
	}
	return o
}

// schemaName is the component name of a struct type: its Go name without the "v1" prefix.
func schemaName(t reflect.Type) string {
	if n, ok := schemaNames[t]; ok {
		return n
	}
	return strings.TrimPrefix(t.Name(), "v1")
}

// openAPI builds the OpenAPI document. serverURL is where the API is reached (the base path
// when served by a running instance).
func openAPI(programVersion, serverURL string) map[string]any {
	s := &schemaSet{defs: map[string]any{}}
	problem := s.ref(reflect.TypeOf(v1Problem{}))
	paths := map[string]any{}
	for _, r := range v1Routes(nil) {
		op := map[string]any{
			"operationId": r.ID,
			"summary":     r.Summary,
			"tags":        []string{r.Tag},
		}
		if r.Description != "" {
			op["description"] = r.Description
		}
		var params []any
		for _, seg := range strings.Split(r.Path, "/") {
			if strings.HasPrefix(seg, "{") {
				n := strings.Trim(seg, "{}")
				params = append(params, map[string]any{
					"name": n, "in": "path", "required": true,
					"schema": map[string]any{"type": "string"}, "description": pathParamDocs[n],
				})
			}
		}
		if len(params) > 0 {
			op["parameters"] = params
		}
		if r.Body != nil {
			op["requestBody"] = map[string]any{
				"required": true,
				"content":  map[string]any{"application/json": map[string]any{"schema": s.ref(reflect.TypeOf(r.Body))}},
			}
		}
		resp := map[string]any{}
		switch {
		case r.Stream:
			resp["200"] = map[string]any{
				"description": r.RespDoc,
				"content": map[string]any{"text/event-stream": map[string]any{
					"schema":       map[string]any{"type": "string"},
					"x-event-data": s.ref(reflect.TypeOf(r.Resp)), // schema of each event's data line
				}},
			}
		case r.Resp != nil:
			resp["200"] = map[string]any{
				"description": r.RespDoc,
				"content":     map[string]any{"application/json": map[string]any{"schema": s.ref(reflect.TypeOf(r.Resp))}},
			}
		}
		for _, code := range r.Errors {
			resp[itoa(code)] = map[string]any{
				"description": errorDocs[code],
				"content":     map[string]any{"application/problem+json": map[string]any{"schema": problem}},
			}
		}
		op["responses"] = resp
		if r.Public {
			op["security"] = []any{}
		}
		item, _ := paths[r.Path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[r.Path] = item
		}
		item[strings.ToLower(r.Method)] = op
	}
	info := map[string]any{
		"title":       "rigctlWeb API",
		"version":     apiDocVersion,
		"summary":     "Control and monitor amateur radios through Hamlib's rigctld.",
		"description": apiDescription,
		"license":     map[string]any{"name": "MIT", "identifier": "MIT"},
	}
	if programVersion != "" {
		info["x-program-version"] = programVersion
	}
	return map[string]any{
		"openapi": openAPIVersion,
		"info":    info,
		"servers": []any{map[string]any{"url": serverURL}},
		"tags": []any{
			map[string]any{"name": "About", "description": "Health, version and the integration handshake."},
			map[string]any{"name": "Radios", "description": "List radios and read their state and capabilities."},
			map[string]any{"name": "Control", "description": "Change a radio's settings. Nothing here can make the radio transmit."},
		},
		"security": []any{
			map[string]any{"bearerAuth": []any{}},
			map[string]any{"cookieAuth": []any{}},
			map[string]any{"hamstackProxy": []any{}},
		},
		"paths": paths,
		"components": map[string]any{
			"schemas": s.defs,
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{"type": "http", "scheme": "bearer",
					"description": "The access token from rigctlWeb's config file (printed on first run, or with --show-token)."},
				"cookieAuth": map[string]any{"type": "apiKey", "in": "cookie", "name": cookieName,
					"description": "Set by the web UI after sign-in."},
				"hamstackProxy": map[string]any{"type": "apiKey", "in": "header", "name": "X-Hamstack-Proxy-Auth",
					"description": "Only in hamstack integration mode, where it replaces the token. Added by hamstack's proxy."},
			},
		},
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

var errorDocs = map[int]string{
	400: "The request is malformed or a value is not valid for this radio.",
	401: "No valid token or session.",
	403: "Not allowed: the radio is read-only, or the setting is one this API never changes because it can make the radio transmit.",
	404: "No radio with that id.",
	422: "The radio or its Hamlib backend does not support this.",
	502: "rigctld or the radio reported an error.",
	503: "The radio is not connected yet.",
	504: "The radio did not answer in time.",
}

var pathParamDocs = map[string]string{
	"id":   "Radio id, as listed by GET /api/v1/rigs.",
	"name": "Hamlib name, as listed in the radio's capabilities (for example RFPOWER or NB).",
}

const apiDescription = `The published, versioned API of rigctlWeb. Anything under /api/v1 follows these rules:

- Within v1, fields and routes are only added. Nothing is renamed or removed, and no field changes meaning.
- Clients must ignore fields they do not know.
- A breaking change gets a new path (/api/v2), and v1 keeps working alongside it for at least two minor releases.

Every route except ` + "`GET /api/health`" + ` needs the access token (` + "`Authorization: Bearer <token>`" + `) or the web UI's session cookie.
Errors are RFC 9457 problem details (application/problem+json).

**Transmitting is not part of v1.** No route keys the transmitter, starts a tune cycle, or turns on VOX, break-in,
tone burst, memory sending or the automatic tuner; functions are an allow-list of receive-side and setup ones. Keying stays with clients that hold a live connection, so that a client that goes away
leaves the radio unkeyed.

Routes not listed here (/api/rigs, /api/views, /ws and so on) are private to rigctlWeb's own web UI and change without
notice.`
