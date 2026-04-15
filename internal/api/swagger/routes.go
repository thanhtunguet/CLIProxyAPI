package swagger

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

var (
	routeParamRegex   = regexp.MustCompile(`:([A-Za-z0-9_]+)`)
	routeWildcardExpr = regexp.MustCompile(`\*([A-Za-z0-9_]+)`)
	handlerMethodExpr = regexp.MustCompile(`\(\*([A-Za-z0-9_]+)\)\.([A-Za-z0-9_]+)`)
	handlerFuncExpr   = regexp.MustCompile(`\.([A-Za-z0-9_]+)(?:-fm)?$`)
)

// RegisterRoutes adds dynamic OpenAPI JSON and Swagger UI routes.
// The OpenAPI document is generated at request time from Gin's route table.
func RegisterRoutes(engine *gin.Engine, sourceRoot string) {
	if engine == nil {
		return
	}

	scanner := newRequestDTOScanner(sourceRoot)

	engine.GET("/openapi.json", func(c *gin.Context) {
		c.JSON(http.StatusOK, buildOpenAPIDoc(engine, scanner))
	})

	handler := ginSwagger.WrapHandler(
		swaggerfiles.Handler,
		ginSwagger.URL("/openapi.json"),
		ginSwagger.DocExpansion("list"),
		ginSwagger.DefaultModelsExpandDepth(-1),
	)
	engine.GET("/swagger/*any", handler)
}

func buildOpenAPIDoc(engine *gin.Engine, scanner *requestDTOScanner) map[string]any {
	doc := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "CLI Proxy API",
			"version":     "v6",
			"description": "Auto-generated from Gin routes and detected request DTOs.",
		},
		"servers": []map[string]any{{"url": "/"}},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"BearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "APIKey",
				},
			},
		},
		"paths": map[string]any{},
	}

	routes := engine.Routes()
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})

	paths := doc["paths"].(map[string]any)
	for _, route := range routes {
		if route.Path == "/openapi.json" || strings.HasPrefix(route.Path, "/swagger/") {
			continue
		}

		openAPIPath := normalizeRoutePath(route.Path)
		pathItem, ok := paths[openAPIPath].(map[string]any)
		if !ok {
			pathItem = map[string]any{}
			paths[openAPIPath] = pathItem
		}

		handlerKeys := routeHandlerKeys(route.Handler)
		schema, hasDTO := scanner.requestSchema(handlerKeys)

		operation := map[string]any{
			"summary":     routeSummary(route),
			"operationId": operationID(route.Method, openAPIPath),
			"tags":        []string{routeTag(route.Path)},
			"responses": map[string]any{
				"200": map[string]any{
					"description": "OK",
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": genericObjectSchema(),
						},
					},
				},
				"default": map[string]any{
					"description": "Error",
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": genericObjectSchema(),
						},
					},
				},
			},
		}

		if !isPublicRoute(route.Path) {
			operation["security"] = []map[string]any{{"BearerAuth": []string{}}}
		}

		params := routePathParameters(openAPIPath)
		if len(params) > 0 {
			operation["parameters"] = params
		}

		if methodSupportsBody(route.Method) {
			requestSchema := genericObjectSchema()
			if hasDTO {
				requestSchema = schema
			}
			operation["requestBody"] = map[string]any{
				"required": hasDTO,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": requestSchema,
					},
				},
			}
		}

		pathItem[strings.ToLower(route.Method)] = operation
	}

	return doc
}

func normalizeRoutePath(path string) string {
	converted := routeParamRegex.ReplaceAllString(path, `{$1}`)
	converted = routeWildcardExpr.ReplaceAllString(converted, `{$1}`)
	return converted
}

func routeSummary(route gin.RouteInfo) string {
	if route.Handler == "" {
		return fmt.Sprintf("%s %s", route.Method, normalizeRoutePath(route.Path))
	}
	keys := routeHandlerKeys(route.Handler)
	if len(keys) == 0 {
		return fmt.Sprintf("%s %s", route.Method, normalizeRoutePath(route.Path))
	}
	return fmt.Sprintf("%s %s", route.Method, keys[0])
}

func operationID(method, path string) string {
	clean := strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_", ".", "_").Replace(path)
	clean = strings.Trim(clean, "_")
	if clean == "" {
		clean = "root"
	}
	return strings.ToLower(method) + "_" + clean
}

func routeTag(path string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return "root"
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "root"
	}
	return parts[0]
}

func isPublicRoute(path string) bool {
	public := []string{
		"/",
		"/healthz",
		"/management.html",
		"/anthropic/callback",
		"/codex/callback",
		"/google/callback",
		"/iflow/callback",
		"/antigravity/callback",
		"/openapi.json",
	}
	for _, item := range public {
		if path == item {
			return true
		}
	}
	return strings.HasPrefix(path, "/swagger/")
}

func routePathParameters(openAPIPath string) []map[string]any {
	segments := strings.Split(openAPIPath, "/")
	params := make([]map[string]any, 0)
	for _, seg := range segments {
		if len(seg) < 3 || seg[0] != '{' || seg[len(seg)-1] != '}' {
			continue
		}
		name := seg[1 : len(seg)-1]
		if name == "" {
			continue
		}
		params = append(params, map[string]any{
			"name":     name,
			"in":       "path",
			"required": true,
			"schema": map[string]any{
				"type": "string",
			},
		})
	}
	return params
}

func methodSupportsBody(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func genericObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	}
}

func routeHandlerKeys(handler string) []string {
	trimmed := strings.TrimSpace(handler)
	if trimmed == "" {
		return nil
	}

	seen := make(map[string]struct{})
	keys := make([]string, 0, 2)
	if match := handlerMethodExpr.FindStringSubmatch(trimmed); len(match) == 3 {
		k := match[1] + "." + match[2]
		seen[k] = struct{}{}
		keys = append(keys, k)
		seen[match[2]] = struct{}{}
		keys = append(keys, match[2])
	}
	if match := handlerFuncExpr.FindStringSubmatch(trimmed); len(match) == 2 {
		if _, ok := seen[match[1]]; !ok {
			keys = append(keys, match[1])
		}
	}
	if len(keys) == 0 {
		return []string{trimmed}
	}
	return keys
}

type requestDTOScanner struct {
	root      string
	once      sync.Once
	byHandler map[string]map[string]any
}

func newRequestDTOScanner(sourceRoot string) *requestDTOScanner {
	root := strings.TrimSpace(sourceRoot)
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		}
	}
	return &requestDTOScanner{
		root:      root,
		byHandler: make(map[string]map[string]any),
	}
}

func (s *requestDTOScanner) requestSchema(handlerKeys []string) (map[string]any, bool) {
	s.once.Do(s.scan)
	for _, key := range handlerKeys {
		if schema, ok := s.byHandler[key]; ok {
			return deepCopyMap(schema), true
		}
	}
	return nil, false
}

func (s *requestDTOScanner) scan() {
	if strings.TrimSpace(s.root) == "" {
		return
	}
	targetDirs := []string{
		"internal/api/handlers/management",
		"sdk/api/handlers/openai",
		"sdk/api/handlers/gemini",
		"sdk/api/handlers/claude",
		"internal/api/modules/amp",
	}
	for _, rel := range targetDirs {
		dir := filepath.Join(s.root, rel)
		s.scanDirectory(dir)
	}
}

func (s *requestDTOScanner) scanDirectory(dir string) {
	if stat, err := os.Stat(dir); err != nil || !stat.IsDir() {
		return
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		name := fi.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, parser.SkipObjectResolution)
	if err != nil {
		return
	}

	for _, pkg := range pkgs {
		typeIndex := collectStructTypeIndex(pkg)
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				receiver := receiverName(fn)
				keys := []string{fn.Name.Name}
				if receiver != "" {
					keys = append([]string{receiver + "." + fn.Name.Name}, keys...)
				}
				schema := detectJSONBindingSchema(fn, typeIndex)
				if schema == nil {
					continue
				}
				for _, key := range keys {
					if _, exists := s.byHandler[key]; exists {
						continue
					}
					s.byHandler[key] = schema
				}
			}
		}
	}
}

func collectStructTypeIndex(pkg *ast.Package) map[string]*ast.StructType {
	index := make(map[string]*ast.StructType)
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				index[typeSpec.Name.Name] = st
			}
		}
	}
	return index
}

func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	t := fn.Recv.List[0].Type
	switch rt := t.(type) {
	case *ast.Ident:
		return rt.Name
	case *ast.StarExpr:
		if id, ok := rt.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func detectJSONBindingSchema(fn *ast.FuncDecl, typeIndex map[string]*ast.StructType) map[string]any {
	localTypes := collectLocalTypes(fn)
	var schema map[string]any
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if schema != nil {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isJSONBindMethod(sel.Sel.Name) || len(call.Args) == 0 {
			return true
		}
		typeExpr := resolveArgTypeExpr(call.Args[0], localTypes)
		if typeExpr == nil {
			return true
		}
		schema = schemaFromTypeExpr(typeExpr, typeIndex, make(map[string]bool))
		return false
	})
	return schema
}

func collectLocalTypes(fn *ast.FuncDecl) map[string]ast.Expr {
	local := make(map[string]ast.Expr)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.DeclStmt:
			gen, ok := n.Decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				return true
			}
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range valueSpec.Names {
					if name == nil {
						continue
					}
					if valueSpec.Type != nil {
						local[name.Name] = valueSpec.Type
						continue
					}
					if i >= len(valueSpec.Values) {
						continue
					}
					if lit, ok := valueSpec.Values[i].(*ast.CompositeLit); ok {
						local[name.Name] = lit.Type
					}
				}
			}
		case *ast.AssignStmt:
			if n.Tok != token.DEFINE {
				return true
			}
			for i, lhs := range n.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name == "_" || i >= len(n.Rhs) {
					continue
				}
				if lit, ok := n.Rhs[i].(*ast.CompositeLit); ok {
					local[ident.Name] = lit.Type
				}
			}
		}
		return true
	})
	return local
}

func resolveArgTypeExpr(expr ast.Expr, localTypes map[string]ast.Expr) ast.Expr {
	switch v := expr.(type) {
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			return resolveArgTypeExpr(v.X, localTypes)
		}
	case *ast.Ident:
		if t, ok := localTypes[v.Name]; ok {
			return t
		}
	case *ast.CompositeLit:
		return v.Type
	}
	return nil
}

func isJSONBindMethod(name string) bool {
	switch name {
	case "ShouldBindJSON", "BindJSON":
		return true
	default:
		return false
	}
}

func schemaFromTypeExpr(typeExpr ast.Expr, typeIndex map[string]*ast.StructType, seen map[string]bool) map[string]any {
	switch t := typeExpr.(type) {
	case *ast.StructType:
		return schemaFromStructType(t, typeIndex, seen)
	case *ast.Ident:
		if isPrimitiveIdent(t.Name) {
			return primitiveSchema(t.Name)
		}
		if seen[t.Name] {
			return genericObjectSchema()
		}
		st, ok := typeIndex[t.Name]
		if !ok {
			return genericObjectSchema()
		}
		seen[t.Name] = true
		schema := schemaFromStructType(st, typeIndex, seen)
		delete(seen, t.Name)
		return schema
	case *ast.StarExpr:
		return schemaFromTypeExpr(t.X, typeIndex, seen)
	case *ast.ArrayType:
		items := schemaFromTypeExpr(t.Elt, typeIndex, seen)
		return map[string]any{
			"type":  "array",
			"items": items,
		}
	case *ast.MapType:
		valueSchema := schemaFromTypeExpr(t.Value, typeIndex, seen)
		return map[string]any{
			"type":                 "object",
			"additionalProperties": valueSchema,
		}
	case *ast.InterfaceType:
		return genericObjectSchema()
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok && x.Name == "time" && t.Sel != nil && t.Sel.Name == "Time" {
			return map[string]any{"type": "string", "format": "date-time"}
		}
		return genericObjectSchema()
	default:
		return genericObjectSchema()
	}
}

func schemaFromStructType(st *ast.StructType, typeIndex map[string]*ast.StructType, seen map[string]bool) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	properties := schema["properties"].(map[string]any)

	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		name := field.Names[0].Name
		if !isExportedIdentifier(name) {
			continue
		}

		jsonName, _, skip := extractJSONFieldName(field.Tag, name)
		if skip {
			continue
		}
		fieldSchema := schemaFromTypeExpr(field.Type, typeIndex, seen)
		properties[jsonName] = fieldSchema
	}

	if len(properties) == 0 {
		schema["additionalProperties"] = true
	}
	return schema
}

func extractJSONFieldName(tagLit *ast.BasicLit, fallback string) (name string, omitEmpty bool, skip bool) {
	name = lowerFirst(fallback)
	if tagLit == nil {
		return name, false, false
	}
	if tagLit.Kind != token.STRING {
		return name, false, false
	}
	raw, err := strconv.Unquote(tagLit.Value)
	if err != nil {
		return name, false, false
	}
	tag := reflect.StructTag(raw)
	value := tag.Get("json")
	if value == "" {
		return name, false, false
	}
	parts := strings.Split(value, ",")
	if len(parts) > 0 {
		switch strings.TrimSpace(parts[0]) {
		case "-":
			return "", false, true
		case "":
			// keep fallback
		default:
			name = strings.TrimSpace(parts[0])
		}
	}
	for _, p := range parts[1:] {
		if strings.TrimSpace(p) == "omitempty" {
			omitEmpty = true
			break
		}
	}
	return name, omitEmpty, false
}

func primitiveSchema(name string) map[string]any {
	switch strings.TrimSpace(name) {
	case "string":
		return map[string]any{"type": "string"}
	case "bool":
		return map[string]any{"type": "boolean"}
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "byte", "rune":
		return map[string]any{"type": "integer"}
	case "float32", "float64":
		return map[string]any{"type": "number"}
	case "any", "interface{}":
		return genericObjectSchema()
	default:
		return genericObjectSchema()
	}
}

func isPrimitiveIdent(name string) bool {
	switch strings.TrimSpace(name) {
	case "string", "bool", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "byte", "rune", "float32", "float64", "any", "interface{}":
		return true
	default:
		return false
	}
}

func isExportedIdentifier(name string) bool {
	if name == "" {
		return false
	}
	r := rune(name[0])
	return r >= 'A' && r <= 'Z'
}

func lowerFirst(v string) string {
	if v == "" {
		return v
	}
	return strings.ToLower(v[:1]) + v[1:]
}

func deepCopyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = deepCopyValue(v)
	}
	return out
}

func deepCopySlice(src []any) []any {
	out := make([]any, 0, len(src))
	for _, v := range src {
		out = append(out, deepCopyValue(v))
	}
	return out
}

func deepCopyValue(v any) any {
	switch value := v.(type) {
	case map[string]any:
		return deepCopyMap(value)
	case []any:
		return deepCopySlice(value)
	case []string:
		out := make([]string, len(value))
		copy(out, value)
		return out
	default:
		return value
	}
}
