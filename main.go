package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/token"
	"go/types"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"golang.org/x/tools/go/packages"
)

var (
	lockFieldStr = flag.String("lockField", "", "mutex lock that will be used for locking callback fields")
	typeNamesStr = flag.String("type", "", "comma-separated list of type names; must be set")
	outputStdout = flag.Bool("stdout", false, "output generated content to the stdout")
	output       = flag.String("output", "", "output file name; default srcdir/<type>_string.go")
	buildTags    = flag.String("tags", "", "comma-separated list of build tags to apply")
)

// File holds a single parsed file and associated data.
type File struct {
	pkg  *Package  // Package to which this file belongs.
	file *ast.File // Parsed AST.
}

type Package struct {
	name  string
	pkg   *packages.Package
	defs  map[*ast.Ident]types.Object
	files []*File
}

type Field struct {
	File           *ast.File
	StructName     string
	StructTypeName string
	StructType     *types.Struct

	// FieldName = snapshotCallbacks
	FieldName string

	LockField *string

	// snapshot
	EventName           string
	CallbackElementType types.Type
	CallbackSliceType   *types.Slice

	CallbackMapType    *types.Map
	CallbackMapKeyType *types.Named

	IsSlice    bool
	IsMapSlice bool
}

func paramsTuple(a types.Type) *types.Tuple {

	switch a := a.(type) {

	// pure signature callback
	case *types.Signature:
		return a.Params()

	// named type callback
	case *types.Named:
		return paramsTuple(a.Underlying())

	default:
		return nil

	}
}

func (f Field) CallbackParamsTuple() *types.Tuple {
	return paramsTuple(f.CallbackElementType)
}

func (f Field) CallbackParamsVarNames() (names []string) {
	tuple := paramsTuple(f.CallbackElementType)
	for i := 0; i < tuple.Len(); i++ {
		v := tuple.At(i)
		names = append(names, v.Name())
	}

	return names
}

func (f Field) CallbackTypeName(qf types.Qualifier) string {
	switch callbackType := f.CallbackElementType.(type) {

	// pure signature callback
	case *types.Signature:
		return types.TypeString(callbackType, qf)

	// named type callback
	case *types.Named:
		return callbackType.Obj().Name()

	default:
		return types.TypeString(callbackType, qf)

	}
}

type Generator struct {
	buf bytes.Buffer // Accumulated output.
	pkg *Package     // Package we are scanning.

	callbackFields []Field

	structTypeReceiverNames map[string]string
}

func (g *Generator) parsePackage(patterns []string, tags []string) {
	cfg := &packages.Config{
		Mode: packages.LoadSyntax,
		// TODO: Need to think about constants in test files. Maybe write type_string_test.go
		// in a separate pass? For later.
		Tests:      false,
		BuildFlags: []string{fmt.Sprintf("-tags=%s", strings.Join(tags, " "))},
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		log.Fatal(err)
	}
	if len(pkgs) != 1 {
		log.Fatalf("error: %d packages found", len(pkgs))
	}
	g.addPackage(pkgs[0])
}

// addPackage adds a type checked Package and its syntax files to the generator.
func (g *Generator) addPackage(pkg *packages.Package) {
	g.pkg = &Package{
		name:  pkg.Name,
		pkg:   pkg,
		defs:  pkg.TypesInfo.Defs,
		files: make([]*File, len(pkg.Syntax)),
	}

	for i, file := range pkg.Syntax {
		g.pkg.files[i] = &File{
			file: file,
			pkg:  g.pkg,
		}
	}
}

func (g *Generator) Newline() {
	fmt.Fprint(&g.buf, "\n")
}

func (g *Generator) Printf(format string, args ...interface{}) {
	fmt.Fprintf(&g.buf, format, args...)
}

func (g *Generator) generate(typeName string) {
	// collect the fields and types
	for _, file := range g.pkg.files {
		// Set the state for this run of the walker.
		if file.file == nil {
			continue
		}

		ast.Inspect(file.file, func(node ast.Node) bool {
			switch decl := node.(type) {

			case *ast.ImportSpec:
				log.Printf("imports: %+v", decl)

			case *ast.FuncDecl:
				// skip functions that don't have receiver
				if decl.Recv == nil {
					return false
				}

				if len(decl.Recv.List) == 0 {
					return false
				}

				recv := decl.Recv.List[0]

				recvTV, ok := g.pkg.pkg.TypesInfo.Types[recv.Type]
				if !ok {
					return true
				}

				switch v := recvTV.Type.(type) {
				case *types.Named:
					g.structTypeReceiverNames[v.String()] = recv.Names[0].String()

				case *types.Pointer:
					g.structTypeReceiverNames[v.Elem().String()] = recv.Names[0].String()
				}

				return true

			case *ast.GenDecl:
				if decl.Tok != token.TYPE {
					// We only care about const declarations.
					return true
				}

				eventNameRE := regexp.MustCompile(`(?:By(\w+))?Callbacks$`)

				for _, spec := range decl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						return true
					}

					if typeSpec.Name.Name != typeName {
						return true
					}

					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						return false
					}

					typeDef := g.pkg.pkg.TypesInfo.Defs[typeSpec.Name]
					fullTypeName := typeDef.Type().String()

					for _, field := range structType.Fields.List {
						// skip field names that are not with the "Callbacks" suffix
						var eventNames []string
						for _, name := range field.Names {
							if matched, err := regexp.MatchString("Callbacks$", name.Name); err == nil && !matched {
								continue
							}

							eventNames = append(eventNames, eventNameRE.ReplaceAllString(name.Name, ""))
						}

						tv, ok := g.pkg.pkg.TypesInfo.Types[field.Type]
						if !ok {
							continue
						}

						// skip fields that are not slice type
						isSlice := false
						isMapSlice := false

						var callbackSliceType *types.Slice = nil
						var callbackMapType *types.Map = nil
						var callbackMapKeyType *types.Named = nil

						switch a := tv.Type.(type) {

						case *types.Slice:
							isSlice = true
							callbackSliceType = a

						case *types.Map:

							callbackMapKeyType, ok = a.Key().(*types.Named)
							if !ok {
								continue
							}

							mapElemSlice, ok := a.Elem().(*types.Slice)
							if !ok {
								continue
							}
							callbackSliceType = mapElemSlice
							callbackMapType = a
							isMapSlice = true

						default:
							log.Printf("%v not a slice type or map[string]slice type", field.Names)
							continue

						}

						for _, eventName := range eventNames {
							g.callbackFields = append(g.callbackFields, Field{
								File:      file.file,
								EventName: strings.Title(eventName),

								// short name
								StructName:     typeSpec.Name.String(),
								StructTypeName: fullTypeName,

								FieldName:           field.Names[0].Name,
								CallbackElementType: callbackSliceType.Elem(),

								IsSlice:           isSlice,
								IsMapSlice:        isMapSlice,
								CallbackSliceType: callbackSliceType,

								CallbackMapType:    callbackMapType,
								CallbackMapKeyType: callbackMapKeyType,

								LockField: lockFieldStr,
							})
						}
					}
				}

			default:
				// log.Printf("node: %+v", decl)
				return true
			}

			return true
		})
	}

	conf := types.Config{Importer: importer.Default()}
	pkgReflect, err := conf.Importer.Import("reflect")
	if err != nil {
		log.Fatal(err)
	}

	var usedImports = map[string]*types.Package{
		pkgReflect.Name(): pkgReflect,
	}

	pkgTypes := g.pkg.pkg.Types
	qf := func(other *types.Package) string {
		if pkgTypes == other {
			return "" // same package; unqualified
		}

		// solve imports
		for _, ip := range pkgTypes.Imports() {
			if other == ip {
				usedImports[ip.Name()] = ip
				return ip.Name()
			}
		}

		return other.Path()
	}

	type TemplateArgs struct {
		RecvName       string
		Field          Field
		Qualifier      types.Qualifier
	}

	funcMap := template.FuncMap{
		"camelCase": func(a string) interface{} {
			return strings.ToLower(string(a[0])) + string(a[1:])
		},
		"join": func(sep string, a []string) interface{} {
			return strings.Join(a, sep)
		},
		"tupleString": func(a *types.Tuple) interface{} {
			return TupleString(a, false, qf)
		},
		"typeString": func(a types.Type) interface{} {
			return types.TypeString(a, qf)
		},
	}

	var sliceCallbackTmpl = template.New("slice-callbacks").Funcs(funcMap)
	sliceCallbackTmpl = template.Must(sliceCallbackTmpl.Parse(`

func ( {{- .RecvName }} *{{ .Field.StructName -}} ) On{{- .Field.EventName -}} (cb {{ .Field.CallbackTypeName .Qualifier -}} ) {
	{{ .RecvName }}.{{ .Field.FieldName }} = append({{- .RecvName }}.{{ .Field.FieldName }}, cb)
}

func ( {{- .RecvName }} *{{ .Field.StructName -}} ) Emit{{- .Field.EventName -}} {{ .Field.CallbackParamsTuple | typeString }} {
	for _, cb := range {{ .RecvName }}.{{ .Field.FieldName }} {
		cb({{ .Field.CallbackParamsVarNames  | join ", " }})
	}
}

func ( {{- .RecvName }} *{{ .Field.StructName -}} ) RemoveOn{{- .Field.EventName -}} (needle {{ .Field.CallbackTypeName .Qualifier -}}) (found bool) {

	var newcallbacks {{ .Field.CallbackSliceType | typeString }}
	var fp = reflect.ValueOf(needle).Pointer()
	for _, cb := range {{ .RecvName }}.{{ .Field.FieldName }} {
		if fp == reflect.ValueOf(cb).Pointer() {
			found = true
		} else {
			newcallbacks = append(newcallbacks, cb)
		}
	}

	if found {
		{{ .RecvName }}.{{ .Field.FieldName }}  = newcallbacks
	}

	return found
}

`))

	var mapSliceCallbackTmpl = template.New("map-slice-callbacks").Funcs(funcMap)
	mapSliceCallbackTmpl = template.Must(mapSliceCallbackTmpl.Parse(`

func ( {{- .RecvName }} *{{ .Field.StructName -}} ) On{{- .Field.EventName -}} By {{- .Field.CallbackMapKeyType | typeString -}} (
	{{- .Field.CallbackMapKeyType | typeString | camelCase }} {{ .Field.CallbackMapKeyType | typeString -}}, cb {{ .Field.CallbackTypeName .Qualifier -}}
) {
{{- if gt (len .Field.LockField) 0 }}
	{{ .RecvName }}.{{ .Field.LockField }}.Lock()
	defer {{ .RecvName }}.{{ .Field.LockField }}.Unlock()
{{- end }}

	if {{ .RecvName }}.{{ .Field.FieldName }} == nil {
		{{ .RecvName }}.{{ .Field.FieldName }} = make( {{- .Field.CallbackMapType | typeString -}} )
	}

	{{ .RecvName }}.{{ .Field.FieldName }}[
		{{- .Field.CallbackMapKeyType | typeString | camelCase -}}
	] = append({{- .RecvName }}.{{ .Field.FieldName }}[
		{{- .Field.CallbackMapKeyType | typeString | camelCase -}}
	], cb)
}

func ( {{- .RecvName }} *{{ .Field.StructName -}} ) Emit{{- .Field.EventName -}} By {{- .Field.CallbackMapKeyType | typeString -}} (
			{{- .Field.CallbackMapKeyType | typeString | camelCase }} {{ .Field.CallbackMapKeyType | typeString -}},
			{{- .Field.CallbackParamsTuple | tupleString -}}
) {
	if {{ .RecvName }}.{{ .Field.FieldName }} == nil {
		return
	}

	callbacks, ok := {{ .RecvName }}.{{ .Field.FieldName }}[ {{- .Field.CallbackMapKeyType | typeString | camelCase -}}  ]
	if !ok {
		return
	}
	
	for _, cb := range callbacks {
		cb({{ .Field.CallbackParamsVarNames  | join ", " }})
	}
}

func ( {{- .RecvName }} *{{ .Field.StructName -}} ) RemoveOn{{- .Field.EventName -}} By {{- .Field.CallbackMapKeyType | typeString -}} (
	{{- .Field.CallbackMapKeyType | typeString | camelCase }} {{ .Field.CallbackMapKeyType | typeString -}}, needle {{ .Field.CallbackTypeName .Qualifier -}}
) (found bool) {

	callbacks, ok := {{ .RecvName }}.{{ .Field.FieldName }}[ {{- .Field.CallbackMapKeyType | typeString | camelCase -}}  ]
	if !ok {
		return
	}
	
	var newcallbacks {{ .Field.CallbackSliceType | typeString }}
	var fp = reflect.ValueOf(needle).Pointer()
	for _, cb := range callbacks {
		if fp == reflect.ValueOf(cb).Pointer() {
			found = true
		} else {
			newcallbacks = append(newcallbacks, cb)
		}
	}

	if found {
		{{ .RecvName }}.{{ .Field.FieldName }}[ {{- .Field.CallbackMapKeyType | typeString | camelCase -}}  ] = newcallbacks
	}

	return found
}
`))

	// scan imports in the first run
	for _, field := range g.callbackFields {
		types.TypeString(field.CallbackParamsTuple(), qf)
	}

	g.Printf("import (")
	g.Newline()
	for _, importedPkg := range usedImports {
		g.Printf("\t%q", importedPkg.Path())
		g.Newline()
	}
	g.Printf(")")
	g.Newline()

	for _, field := range g.callbackFields {
		recvName, ok := g.structTypeReceiverNames[field.StructTypeName]
		if !ok {
			recvName = string(field.StructName[0])
		}

		var t *template.Template

		if field.IsMapSlice {
			t = mapSliceCallbackTmpl
		} else if field.IsSlice {
			t = sliceCallbackTmpl
		} else {
			log.Fatal("unexpected field type")
		}

		err := t.Execute(&g.buf, TemplateArgs{
			Field:          field,
			RecvName:       recvName,
			Qualifier:      qf,
		})
		if err != nil {
			log.Fatal(err)
		}
	}
}

func (g *Generator) format() []byte {
	src, err := format.Source(g.buf.Bytes())
	if err != nil {
		// Should never happen, but can arise when developing this code.
		// The user can compile the output to see the error.
		log.Printf("warning: internal error: invalid Go generated: %s", err)
		log.Printf("warning: compile the package to analyze the error")
		return g.buf.Bytes()
	}
	return src
}

// isDirectory reports whether the named file is a directory.
func isDirectory(name string) bool {
	info, err := os.Stat(name)
	if err != nil {
		log.Fatal(err)
	}
	return info.IsDir()
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("callbackgen: ")
	flag.Parse()
	if len(*typeNamesStr) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	typeNames := strings.Split(*typeNamesStr, ",")
	var tags []string
	if len(*buildTags) > 0 {
		tags = strings.Split(*buildTags, ",")
	}

	// We accept either one directory or a list of files. Which do we have?
	args := flag.Args()
	if len(args) == 0 {
		// Default: process whole package in current directory.
		args = []string{"."}
	}

	// Parse the package once.
	var dir string

	// TODO(suzmue): accept other patterns for packages (directories, list of files, import paths, etc).
	if len(args) == 1 && isDirectory(args[0]) {
		dir = args[0]
	} else {
		if len(tags) != 0 {
			log.Fatal("-tags option applies only to directories, not when files are specified")
		}
		dir = filepath.Dir(args[0])
	}

	g := Generator{
		structTypeReceiverNames: map[string]string{},
	}

	g.parsePackage(args, tags)

	g.Printf("// Code generated by \"callbackgen %s\"; DO NOT EDIT.\n", strings.Join(os.Args[1:], " "))
	g.Newline()
	g.Newline()
	g.Printf("package %s", g.pkg.name)
	g.Newline()
	g.Newline()

	for _, typeName := range typeNames {
		g.generate(typeName)
	}

	// Format the output.
	src := g.format()

	var err error
	if *outputStdout {
		_, err = fmt.Fprint(os.Stdout, string(src))
	} else {
		// Write to file.
		outputName := *output
		if outputName == "" {
			baseName := fmt.Sprintf("%s_callbacks.go", typeNames[0])
			outputName = filepath.Join(dir, strings.ToLower(baseName))
		}
		err = ioutil.WriteFile(outputName, src, 0644)
	}

	if err != nil {
		log.Fatalf("writing output: %s", err)
	}
}

func TupleString(tup *types.Tuple, variadic bool, qf types.Qualifier) string {
	buf := bytes.NewBuffer(nil)
	// buf.WriteByte('(')
	if tup != nil {

		for i := 0; i < tup.Len(); i++ {
			v := tup.At(i)
			if i > 0 {
				buf.WriteString(", ")
			}

			name := v.Name()
			if name != "" {
				buf.WriteString(name)
				buf.WriteByte(' ')
			}

			typ := v.Type()

			if variadic && i == tup.Len()-1 {
				if s, ok := typ.(*types.Slice); ok {
					buf.WriteString("...")
					typ = s.Elem()
				} else {
					// special case:
					// append(s, "foo"...) leads to signature func([]byte, string...)
					if t, ok := typ.Underlying().(*types.Basic); !ok || t.Kind() != types.String {
						panic("internal error: string type expected")
					}
					types.WriteType(buf, typ, qf)
					buf.WriteString("...")
					continue
				}
			}
			types.WriteType(buf, typ, qf)
		}
	}
	// buf.WriteByte(')')
	return buf.String()
}
