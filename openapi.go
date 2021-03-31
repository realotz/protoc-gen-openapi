package main

import (
	"encoding/json"
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gopkg.in/yaml.v2"
	"net/http"
	"strings"
)

var methodSets = make(map[string]int)

type serviceDesc struct {
	ServiceType string // Greeter
	Comments    string // Comments
	ServiceName string // helloworld.Greeter
	Metadata    string // api/helloworld/helloworld.proto
	Methods     []*methodDesc
	MethodSets  map[string]*methodDesc
}

type methodDesc struct {
	// method
	Name    string
	Num     int
	Vars    []string
	Forms   []string
	Request string
	Reply   string
	// http_rule
	Path         string
	Method       string
	Body         string
	ResponseBody string
}

func Marshal(fullname protoreflect.FullName) protoreflect.FullName {
	name := string(fullname)
	if name == "" {
		return ""
	}
	temp := strings.Split(name, ".")
	var s string
	for _, v := range temp {
		vv := []rune(v)
		if len(vv) > 0 {
			if bool(vv[0] >= 'a' && vv[0] <= 'z') { //首字母大写
				vv[0] -= 32
			}
			s += string(vv)
		}
	}
	return protoreflect.FullName(s)
}

// generateFile generates a _http.pb.go file containing kratos errors definitions.
func generateFile(gen *protogen.Plugin, file *protogen.File) *protogen.GeneratedFile {
	if len(file.Services) == 0 {
		return nil
	}
	filename := file.GeneratedFilenamePrefix + "_openapi.yaml"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)
	swagger := openapi3.Swagger{
		OpenAPI: "3.0.0",
		Components: openapi3.Components{
			Schemas:    make(openapi3.Schemas),
			Parameters: make(openapi3.ParametersMap),
		},
		Info: &openapi3.Info{
			Title:          string(file.Desc.FullName()),
			Description:    string(file.Desc.FullName()),
			TermsOfService: "",
			Contact:        nil,
			License:        nil,
			Version:        string(file.Desc.Package().Name()),
		},
		Paths:    make(openapi3.Paths, 0),
		Security: nil,
		Servers:  nil,
		Tags:     make(openapi3.Tags, 0, len(file.Services)),
	}
	genTags(&swagger, file)
	genComponents(&swagger, file)
	genPaths(&swagger, file)
	jsonData, err := swagger.MarshalJSON()
	if err != nil {
		return nil
	}
	var inter interface{}
	_ = json.Unmarshal(jsonData, &inter)
	jsonData, err = yaml.Marshal(inter)
	if err != nil {
		return nil
	}
	g.P(string(jsonData))
	return g
}

func genComponents(swagger *openapi3.Swagger, file *protogen.File) {
	for _, msg := range file.Messages {
		schema := openapi3.NewObjectSchema()
		schema.Properties = make(openapi3.Schemas)
		for _, filed := range msg.Fields {
			s := createSchema(filed.Desc)
			s.Description = commentDesc(filed.Comments)
			schema.Properties[string(filed.Desc.Name())] = &openapi3.SchemaRef{
				Value: createSchema(filed.Desc),
			}
		}
		swagger.Components.Schemas[string(Marshal(msg.Desc.FullName()))] = &openapi3.SchemaRef{
			Value: schema,
		}
	}
}

func createSchema(desc protoreflect.FieldDescriptor) *openapi3.Schema {
	var schema *openapi3.Schema
	if desc.IsMap() {
		schema = openapi3.NewObjectSchema()
		return schema
	}
	if desc.IsList() {
		schema = openapi3.NewArraySchema()
		return openapi3.NewArraySchema()
	}
	switch desc.Kind() {
	case protoreflect.BoolKind:
		schema = openapi3.NewBoolSchema()
		break
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Uint32Kind:
		schema = openapi3.NewInt32Schema()
		break
	case protoreflect.Uint64Kind, protoreflect.Int64Kind, protoreflect.Sint64Kind:
		schema = openapi3.NewInt64Schema()
		break
	case protoreflect.Sfixed32Kind, protoreflect.Fixed32Kind, protoreflect.FloatKind,
		protoreflect.Sfixed64Kind, protoreflect.Fixed64Kind, protoreflect.DoubleKind:
		schema = openapi3.NewFloat64Schema()
		break
	case protoreflect.MessageKind:
		schema = openapi3.NewObjectSchema()
		schema.Properties = make(openapi3.Schemas)
		for i := 0; i < desc.Message().Fields().Len(); i++ {
			f := desc.Message().Fields().Get(i)
			schema.Properties[string(f.Name())] = &openapi3.SchemaRef{
				Value: createSchema(f),
			}
		}
		break
	case protoreflect.EnumKind:
		schema = openapi3.NewSchema()
		var enum []interface{}
		for i := 0; i < desc.Enum().Values().Len(); i++ {
			e := desc.Enum().Values().Get(i)
			enum = append(enum, e.Name())
		}
		schema.Enum = enum
		break
	default:
		schema = openapi3.NewStringSchema()
		break
	}
	return schema
}

func commentDesc(comment protogen.CommentSet) string {
	str := comment.Leading.String()
	str = strings.ReplaceAll(str, "//", "")
	str = strings.ReplaceAll(str, "\n", "")
	return strings.TrimSpace(str)
}

func genPaths(swagger *openapi3.Swagger, file *protogen.File) {
	file.Extensions = append(file.Extensions)
	for _, service := range file.Services {
		// HTTP Server.
		sd := &serviceDesc{
			ServiceType: service.GoName,
			ServiceName: string(service.Desc.FullName()),
			Metadata:    file.Desc.Path(),
		}
		for _, method := range service.Methods {
			rule, ok := proto.GetExtension(method.Desc.Options(), annotations.E_Http).(*annotations.HttpRule)
			if rule != nil && ok {
				for _, bind := range rule.AdditionalBindings {
					sd.Methods = append(sd.Methods, buildHTTPRule(method, bind))
				}
				sd.Methods = append(sd.Methods, buildHTTPRule(method, rule))
			} else {
				path := fmt.Sprintf("/%s/%s", service.Desc.FullName(), method.Desc.Name())
				sd.Methods = append(sd.Methods, buildMethodDesc(method, "POST", path))
			}
		}
		for k, method := range sd.Methods {
			item := &openapi3.PathItem{
				Summary:     method.Name,
				Description: commentDesc(service.Methods[k].Comments),
			}
			ok := "ok"

			op := &openapi3.Operation{
				Tags:        []string{string(service.Desc.FullName())},
				Summary:     commentDesc(service.Methods[k].Comments),
				Description: commentDesc(service.Methods[k].Comments),
				Responses: openapi3.Responses{
					"200": {
						Ref: "",
						Value: &openapi3.Response{
							Description: &ok,
							Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{
								Ref: fmt.Sprintf("#/components/schemas/%s%s", Marshal(file.Desc.Package()), method.Reply),
							}),
						},
					},
				},
			}
			reqBody := &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Description: commentDesc(service.Methods[k].Input.Comments),
					Required:    true,
					Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{
						Ref: fmt.Sprintf("#/components/schemas/%s%s", Marshal(file.Desc.Package()), method.Request),
					}),
				},
			}
			switch method.Method {
			case http.MethodGet:
				params := openapi3.NewParameters()
				for _, f := range service.Methods[k].Input.Fields {
					param := openapi3.NewQueryParameter(f.Desc.JSONName())
					param.Description = commentDesc(f.Comments)
					param.Schema = openapi3.NewSchemaRef("", openapi3.NewStringSchema())
					params = append(params, &openapi3.ParameterRef{
						Value: param,
					})
				}
				op.Parameters = params
				item.Get = op
			case http.MethodOptions:
				op.RequestBody = reqBody
				item.Options = op
			case http.MethodPut:
				op.RequestBody = reqBody
				item.Put = op
			case http.MethodPatch:
				op.RequestBody = reqBody
				item.Patch = op
			case http.MethodConnect:
				op.RequestBody = reqBody
				item.Connect = op
			case http.MethodTrace:
				op.RequestBody = reqBody
				item.Trace = op
			case http.MethodPost:
				op.RequestBody = reqBody
				item.Post = op
			case http.MethodHead:
				op.RequestBody = reqBody
				item.Head = op
			case http.MethodDelete:
				params := openapi3.NewParameters()
				for _, f := range service.Methods[k].Input.Fields {
					param := openapi3.NewPathParameter(f.Desc.JSONName())
					param.Description = commentDesc(f.Comments)
					param.Schema = openapi3.NewSchemaRef("", openapi3.NewStringSchema())
					params = append(params, &openapi3.ParameterRef{
						Value: param,
					})
				}
				item.Delete = op
			}
			swagger.Paths[method.Path] = item
		}
	}
}

func genTags(swagger *openapi3.Swagger, file *protogen.File) {
	for _, s := range file.Services {
		swagger.Tags = append(swagger.Tags, &openapi3.Tag{
			Name:        string(s.Desc.FullName()),
			Description: commentDesc(s.Comments),
		})
	}
}

func buildHTTPRule(m *protogen.Method, rule *annotations.HttpRule) *methodDesc {
	var (
		path         string
		method       string
		body         string
		responseBody string
	)
	switch pattern := rule.Pattern.(type) {
	case *annotations.HttpRule_Get:
		path = pattern.Get
		method = "GET"
	case *annotations.HttpRule_Put:
		path = pattern.Put
		method = "PUT"
	case *annotations.HttpRule_Post:
		path = pattern.Post
		method = "POST"
	case *annotations.HttpRule_Delete:
		path = pattern.Delete
		method = "DELETE"
	case *annotations.HttpRule_Patch:
		path = pattern.Patch
		method = "PATCH"
	case *annotations.HttpRule_Custom:
		path = pattern.Custom.Path
		method = pattern.Custom.Kind
	}
	body = rule.Body
	responseBody = rule.ResponseBody
	md := buildMethodDesc(m, method, path)
	if body != "" {
		md.Body = "." + camelCaseVars(body)
	}
	if responseBody != "" {
		md.ResponseBody = "." + camelCaseVars(responseBody)
	}

	return md
}

func buildMethodDesc(m *protogen.Method, method, path string) *methodDesc {
	defer func() { methodSets[m.GoName]++ }()
	return &methodDesc{
		Name:    m.GoName,
		Num:     methodSets[m.GoName],
		Request: m.Input.GoIdent.GoName,
		Reply:   m.Output.GoIdent.GoName,
		Path:    path,
		Method:  method,
		Vars:    buildPathVars(m, path),
	}
}

func buildPathVars(method *protogen.Method, path string) (res []string) {
	for _, v := range strings.Split(path, "/") {
		if strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}") {
			name := strings.TrimRight(strings.TrimLeft(v, "{"), "}")
			res = append(res, name)
		}
	}
	return
}

func camelCaseVars(s string) string {
	var (
		vars []string
		subs = strings.Split(s, ".")
	)
	for _, sub := range subs {
		vars = append(vars, camelCase(sub))
	}
	return strings.Join(vars, ".")
}

// camelCase returns the CamelCased name.
// If there is an interior underscore followed by a lower case letter,
// drop the underscore and convert the letter to upper case.
// There is a remote possibility of this rewrite causing a name collision,
// but it's so remote we're prepared to pretend it's nonexistent - since the
// C++ generator lowercases names, it's extremely unlikely to have two fields
// with different capitalizations.
// In short, _my_field_name_2 becomes XMyFieldName_2.
func camelCase(s string) string {
	if s == "" {
		return ""
	}
	t := make([]byte, 0, 32)
	i := 0
	if s[0] == '_' {
		// Need a capital letter; drop the '_'.
		t = append(t, 'X')
		i++
	}
	// Invariant: if the next letter is lower case, it must be converted
	// to upper case.
	// That is, we process a word at a time, where words are marked by _ or
	// upper case letter. Digits are treated as words.
	for ; i < len(s); i++ {
		c := s[i]
		if c == '_' && i+1 < len(s) && isASCIILower(s[i+1]) {
			continue // Skip the underscore in s.
		}
		if isASCIIDigit(c) {
			t = append(t, c)
			continue
		}
		// Assume we have a letter now - if not, it's a bogus identifier.
		// The next word is a sequence of characters that must start upper case.
		if isASCIILower(c) {
			c ^= ' ' // Make it a capital letter.
		}
		t = append(t, c) // Guaranteed not lower case.
		// Accept lower case sequence that follows.
		for i+1 < len(s) && isASCIILower(s[i+1]) {
			i++
			t = append(t, s[i])
		}
	}
	return string(t)
}

// Is c an ASCII lower-case letter?
func isASCIILower(c byte) bool {
	return 'a' <= c && c <= 'z'
}

// Is c an ASCII digit?
func isASCIIDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

const deprecationComment = "// Deprecated: Do not use."
