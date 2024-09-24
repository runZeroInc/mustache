package mustache

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// RenderFn is the signature of a function which can be called from a lambda section
type RenderFn func(text string) (string, error)

type Compiler struct {
	partial        PartialProvider
	outputMode     EscapeMode
	errorOnMissing bool
}

func New() *Compiler {
	return &Compiler{}
}

// WithPartials adds a partial provider and enables support for partials.
func (r *Compiler) WithPartials(pp PartialProvider) *Compiler {
	r.partial = pp
	return r
}

// WithEscapeMode sets the output mode to either HTML, JSON or raw (plain text).
// The default is HTML.
func (r *Compiler) WithEscapeMode(m EscapeMode) *Compiler {
	r.outputMode = m
	return r
}

// WithErrors enables errors when there is a missing data object referred to by the template, a missing partial,
// or a missing partial provider to handle a partial. Otherwise, errors are ignored and result in empty strings in the
// output.
func (r *Compiler) WithErrors(b bool) *Compiler {
	r.errorOnMissing = b
	return r
}

// CompileString compiles a Mustache template from a string.
func (r *Compiler) CompileString(data string) (*Template, error) {
	tmpl := Template{data, "{{", "}}", 0, 1, []any{}, false, r.partial, r.outputMode, r.errorOnMissing, r}
	err := tmpl.parse()
	if err != nil {
		return nil, err
	}
	return &tmpl, nil
}

// CompileFile compiles a Mustache template from a file.
func (r *Compiler) CompileFile(filename string) (*Template, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return r.CompileString(string(data))
}

// A TagType represents the specific type of mustache tag that a Tag
// represents. The zero TagType is not a valid type.
type TagType uint

// Defines representing the possible Tag types.
const (
	Invalid TagType = iota
	Variable
	Section
	InvertedSection
	Partial
)

// SkipWhitespaceTagTypes lists the types of tag that will cause all whitespace
// until EOL to be skipped, if the line only contains a tag and whitespace.
const (
	SkipWhitespaceTagTypes = "#^/<>=!"
)

func (t TagType) String() string {
	if int(t) < len(tagNames) {
		return tagNames[t]
	}
	return "type" + strconv.Itoa(int(t))
}

var tagNames = []string{
	Invalid:         "Invalid",
	Variable:        "Variable",
	Section:         "Section",
	InvertedSection: "InvertedSection",
	Partial:         "Partial",
}

// Tag represents the different mustache tag types.
//
// Not all methods apply to all kinds of tags. Restrictions, if any, are noted
// in the documentation for each method. Use the Type method to find out the
// type of tag before calling type-specific methods. Calling a method
// inappropriate to the type of tag causes a run time panic.
type Tag interface {
	// Type returns the type of the tag.
	Type() TagType
	// Name returns the name of the tag.
	Name() string
	// Tags returns any child tags. It panics for tag types which cannot contain
	// child tags (i.e. variable tags).
	Tags() []Tag
}

type textElement struct {
	text []byte
}

type varElement struct {
	name string
	raw  bool
}

type sectionElement struct {
	name      string
	inverted  bool
	startline int
	elems     []any
}

type partialElement struct {
	name   string
	indent string
	prov   PartialProvider
}

// EscapeMode indicates what sort of escaping to perform in template output.
// EscapeHTML is the default, and assumes the template is producing HTML.
// EscapeJSON switches to JSON escaping, for use cases such as generating Slack messages.
// Raw turns off escaping, for situations where you are absolutely sure you want plain text.
type EscapeMode int

const (
	EscapeHTML EscapeMode = iota // Escape output as HTML (default)
	EscapeJSON                   // Escape output as JSON
	Raw                          // Do not escape output (plain text mode)
)

// Template represents a compiled mustache template which can be used to render data.
type Template struct {
	data           string
	otag           string
	ctag           string
	p              int
	curline        int
	elems          []any
	forceRaw       bool
	partial        PartialProvider
	outputMode     EscapeMode
	errorOnMissing bool
	parent         *Compiler
}

type parseError struct {
	line    int
	message string
}

// Tags returns the mustache tags for the given template.
func (tmpl *Template) Tags() []Tag {
	return extractTags(tmpl.elems)
}

func extractTags(elems []any) []Tag {
	tags := make([]Tag, 0, len(elems))
	for _, elem := range elems {
		switch elem := elem.(type) {
		case *varElement:
			tags = append(tags, elem)
		case *sectionElement:
			tags = append(tags, elem)
		case *partialElement:
			tags = append(tags, elem)
		}
	}
	return tags
}

func (e *varElement) Type() TagType {
	return Variable
}

func (e *varElement) Name() string {
	return e.name
}

func (e *varElement) Tags() []Tag {
	panic("mustache: Tags on Variable type")
}

func (e *sectionElement) Type() TagType {
	if e.inverted {
		return InvertedSection
	}
	return Section
}

func (e *sectionElement) Name() string {
	return e.name
}

func (e *sectionElement) Tags() []Tag {
	return extractTags(e.elems)
}

func (e *partialElement) Type() TagType {
	return Partial
}

func (e *partialElement) Name() string {
	return e.name
}

func (e *partialElement) Tags() []Tag {
	return nil
}

func (p parseError) Error() string {
	return fmt.Sprintf("line %d: %s", p.line, p.message)
}

func (tmpl *Template) readString(s string) (string, error) {
	newlines := 0
	for i := tmpl.p; ; i++ {
		// are we at the end of the string?
		if i+len(s) > len(tmpl.data) {
			return tmpl.data[tmpl.p:], io.EOF
		}

		if tmpl.data[i] == '\n' {
			newlines++
		}

		if tmpl.data[i] != s[0] {
			continue
		}

		match := true
		for j := 1; j < len(s); j++ {
			if s[j] != tmpl.data[i+j] {
				match = false
				break
			}
		}

		if match {
			e := i + len(s)
			text := tmpl.data[tmpl.p:e]
			tmpl.p = e

			tmpl.curline += newlines
			return text, nil
		}
	}
}

type textReadingResult struct {
	text          string
	padding       string
	mayStandalone bool
}

func (tmpl *Template) readText() (*textReadingResult, error) {
	pPrev := tmpl.p
	text, err := tmpl.readString(tmpl.otag)
	if err == io.EOF {
		return &textReadingResult{
			text:          text,
			padding:       "",
			mayStandalone: false,
		}, err
	}

	var i int
	for i = tmpl.p - len(tmpl.otag); i > pPrev; i-- {
		if tmpl.data[i-1] != ' ' && tmpl.data[i-1] != '\t' {
			break
		}
	}

	mayStandalone := i == 0 || tmpl.data[i-1] == '\n'

	if mayStandalone {
		return &textReadingResult{
			text:          tmpl.data[pPrev:i],
			padding:       tmpl.data[i : tmpl.p-len(tmpl.otag)],
			mayStandalone: true,
		}, nil
	}

	return &textReadingResult{
		text:          tmpl.data[pPrev : tmpl.p-len(tmpl.otag)],
		padding:       "",
		mayStandalone: false,
	}, nil
}

type tagReadingResult struct {
	tag        string
	standalone bool
}

func (tmpl *Template) readTag(mayStandalone bool) (*tagReadingResult, error) {
	var text string
	var err error
	if tmpl.p < len(tmpl.data) && tmpl.data[tmpl.p] == '{' {
		text, err = tmpl.readString("}" + tmpl.ctag)
	} else {
		text, err = tmpl.readString(tmpl.ctag)
	}

	if err == io.EOF {
		// put the remaining text in a block
		return nil, parseError{tmpl.curline, "unmatched open tag"}
	}

	text = text[:len(text)-len(tmpl.ctag)]

	// trim the close tag off the text
	tag := strings.TrimSpace(text)
	if len(tag) == 0 {
		return nil, parseError{tmpl.curline, "empty tag"}
	}

	eow := tmpl.p
	for i := tmpl.p; i < len(tmpl.data); i++ {
		if !(tmpl.data[i] == ' ' || tmpl.data[i] == '\t') {
			eow = i
			break
		}
	}

	standalone := true
	if mayStandalone {
		if !strings.Contains(SkipWhitespaceTagTypes, tag[0:1]) {
			standalone = false
		} else {
			if eow == len(tmpl.data) {
				standalone = true
				tmpl.p = eow
			} else if eow < len(tmpl.data) && tmpl.data[eow] == '\n' {
				standalone = true
				tmpl.p = eow + 1
				tmpl.curline++
			} else if eow+1 < len(tmpl.data) && tmpl.data[eow] == '\r' && tmpl.data[eow+1] == '\n' {
				standalone = true
				tmpl.p = eow + 2
				tmpl.curline++
			} else {
				standalone = false
			}
		}
	}

	return &tagReadingResult{
		tag:        tag,
		standalone: standalone,
	}, nil
}

func (tmpl *Template) parsePartial(name, indent string) (*partialElement, error) {
	return &partialElement{
		name:   name,
		indent: indent,
		prov:   tmpl.partial,
	}, nil
}

func (tmpl *Template) parseSection(section *sectionElement) error {
	for {
		textResult, err := tmpl.readText()
		text := textResult.text
		padding := textResult.padding
		mayStandalone := textResult.mayStandalone

		if err == io.EOF {
			// put the remaining text in a block
			return parseError{section.startline, "Section " + section.name + " has no closing tag"}
		}

		// put text into an item
		section.elems = append(section.elems, &textElement{[]byte(text)})

		tagResult, err := tmpl.readTag(mayStandalone)
		if err != nil {
			return err
		}

		if !tagResult.standalone {
			section.elems = append(section.elems, &textElement{[]byte(padding)})
		}

		tag := tagResult.tag
		switch tag[0] {
		case '!':
			// ignore comment
			break
		case '#', '^':
			name := strings.TrimSpace(tag[1:])
			se := sectionElement{name, tag[0] == '^', tmpl.curline, []any{}}
			err := tmpl.parseSection(&se)
			if err != nil {
				return err
			}
			section.elems = append(section.elems, &se)
		case '/':
			name := strings.TrimSpace(tag[1:])
			if name != section.name {
				return parseError{tmpl.curline, "interleaved closing tag: " + name}
			}
			return nil
		case '>':
			name := strings.TrimSpace(tag[1:])
			partial, err := tmpl.parsePartial(name, textResult.padding)
			if err != nil {
				return err
			}
			section.elems = append(section.elems, partial)
		case '=':
			if len(tag) < 2 || tag[len(tag)-1] != '=' {
				return parseError{tmpl.curline, "invalid meta tag"}
			}
			tag = strings.TrimSpace(tag[1 : len(tag)-1])
			newtags := strings.SplitN(tag, " ", 2)
			if len(newtags) == 2 {
				tmpl.otag = newtags[0]
				tmpl.ctag = newtags[1]
			}
		case '{':
			if tag[len(tag)-1] == '}' {
				// use a raw tag
				name := strings.TrimSpace(tag[1 : len(tag)-1])
				section.elems = append(section.elems, &varElement{name, true})
			}
		case '&':
			name := strings.TrimSpace(tag[1:])
			section.elems = append(section.elems, &varElement{name, true})
		default:
			section.elems = append(section.elems, &varElement{tag, tmpl.forceRaw})
		}
	}
}

func (tmpl *Template) parse() error {
	for {
		textResult, err := tmpl.readText()
		text := textResult.text
		padding := textResult.padding
		mayStandalone := textResult.mayStandalone

		if err == io.EOF {
			// put the remaining text in a block
			tmpl.elems = append(tmpl.elems, &textElement{[]byte(text)})
			return nil
		}

		// put text into an item
		tmpl.elems = append(tmpl.elems, &textElement{[]byte(text)})

		tagResult, err := tmpl.readTag(mayStandalone)
		if err != nil {
			return err
		}

		if !tagResult.standalone {
			tmpl.elems = append(tmpl.elems, &textElement{[]byte(padding)})
		}

		tag := tagResult.tag
		switch tag[0] {
		case '!':
			// ignore comment
			break
		case '#', '^':
			name := strings.TrimSpace(tag[1:])
			se := sectionElement{name, tag[0] == '^', tmpl.curline, []any{}}
			err := tmpl.parseSection(&se)
			if err != nil {
				return err
			}
			tmpl.elems = append(tmpl.elems, &se)
		case '/':
			return parseError{tmpl.curline, "unmatched close tag"}
		case '>':
			name := strings.TrimSpace(tag[1:])
			partial, err := tmpl.parsePartial(name, textResult.padding)
			if err != nil {
				return err
			}
			tmpl.elems = append(tmpl.elems, partial)
		case '=':
			if tag[len(tag)-1] != '=' || len(tag) < 2 {
				return parseError{tmpl.curline, "Invalid meta tag"}
			}
			tag = strings.TrimSpace(tag[1 : len(tag)-1])
			newtags := strings.SplitN(tag, " ", 2)
			if len(newtags) == 2 {
				tmpl.otag = newtags[0]
				tmpl.ctag = newtags[1]
			}
		case '{':
			// use a raw tag
			if tag[len(tag)-1] == '}' {
				name := strings.TrimSpace(tag[1 : len(tag)-1])
				tmpl.elems = append(tmpl.elems, &varElement{name, true})
			}
		case '&':
			name := strings.TrimSpace(tag[1:])
			tmpl.elems = append(tmpl.elems, &varElement{name, true})
		default:
			tmpl.elems = append(tmpl.elems, &varElement{tag, tmpl.forceRaw})
		}
	}
}

// Evaluate interfaces and pointers looking for a value that can look up the name, via a
// struct field, method, or map key, and return the result of the lookup.
func lookup(contextChain []any, name string, errorOnMissing bool) (reflect.Value, error) {
	// dot notation
	if name != "." && strings.Contains(name, ".") {
		parts := strings.SplitN(name, ".", 2)

		v, err := lookup(contextChain, parts[0], errorOnMissing)
		if err != nil {
			return v, err
		}
		return lookup([]any{v}, parts[1], errorOnMissing)
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Panic while looking up %q: %s\n", name, r)
		}
	}()

Outer:
	for _, ctx := range contextChain {
		v := ctx.(reflect.Value)
		for v.IsValid() {
			typ := v.Type()
			if n := v.Type().NumMethod(); n > 0 {
				for i := 0; i < n; i++ {
					m := typ.Method(i)
					mtyp := m.Type
					if m.Name == name && mtyp.NumIn() == 1 {
						return v.Method(i).Call(nil)[0], nil
					}
				}
			}
			if name == "." {
				return v, nil
			}
			switch av := v; av.Kind() {
			case reflect.Ptr:
				v = av.Elem()
			case reflect.Interface:
				v = av.Elem()
			case reflect.Struct:
				ret := av.FieldByName(name)
				if ret.IsValid() {
					return ret, nil
				}
				continue Outer
			case reflect.Map:
				ret := av.MapIndex(reflect.ValueOf(name))
				if ret.IsValid() {
					return ret, nil
				}
				continue Outer
			default:
				continue Outer
			}
		}
	}
	if !errorOnMissing {
		return reflect.Value{}, nil
	}
	return reflect.Value{}, fmt.Errorf("missing variable %q", name)
}

func isEmpty(v reflect.Value) bool {
	if !v.IsValid() || v.Interface() == nil {
		return true
	}

	valueInd := indirect(v)
	if !valueInd.IsValid() {
		return true
	}
	switch val := valueInd; val.Kind() {
	case reflect.Array, reflect.Slice:
		return val.Len() == 0
	case reflect.String:
		return len(strings.TrimSpace(val.String())) == 0
	default:
		return valueInd.IsZero()
	}
}

func indirect(v reflect.Value) reflect.Value {
loop:
	for v.IsValid() {
		switch av := v; av.Kind() {
		case reflect.Ptr:
			v = av.Elem()
		case reflect.Interface:
			v = av.Elem()
		default:
			break loop
		}
	}
	return v
}

func (tmpl *Template) renderSection(section *sectionElement, contextChain []any, buf io.Writer) error {
	value, err := lookup(contextChain, section.name, tmpl.errorOnMissing)
	if err != nil {
		return err
	}
	context := contextChain[0].(reflect.Value)
	var contexts []any
	// if the value is nil, check if it's an inverted section
	isEmpty := isEmpty(value)
	if isEmpty && !section.inverted || !isEmpty && section.inverted {
		return nil
	} else if !section.inverted {
		valueInd := indirect(value)
		switch val := valueInd; val.Kind() {
		case reflect.Slice:
			for i := 0; i < val.Len(); i++ {
				contexts = append(contexts, val.Index(i))
			}
		case reflect.Array:
			for i := 0; i < val.Len(); i++ {
				contexts = append(contexts, val.Index(i))
			}
		case reflect.Map, reflect.Struct:
			contexts = append(contexts, value)
		case reflect.Func:
			var text bytes.Buffer
			getSectionText(section.elems, &text)
			render := func(text string) (string, error) {
				templ, err := tmpl.parent.CompileString(text)
				if err != nil {
					return "", err
				}
				var buf bytes.Buffer
				err = templ.renderTemplate(contextChain, &buf)
				if err != nil {
					return "", err
				}
				return buf.String(), nil
			}
			in := []reflect.Value{reflect.ValueOf(text.String()), reflect.ValueOf(render)}
			res := val.Call(in)
			resStr := res[0].String()
			if !res[1].IsNil() {
				return res[1].Interface().(error)
			}
			fmt.Fprintf(buf, "%s", resStr)
			return nil
		default:
			// Spec: Non-false sections have their value at the top of context,
			// accessible as {{.}} or through the parent context. This gives
			// a simple way to display content conditionally if a variable exists.
			contexts = append(contexts, value)
		}
	} else if section.inverted {
		contexts = append(contexts, context)
	}

	chain2 := make([]any, len(contextChain)+1)
	copy(chain2[1:], contextChain)
	// by default we execute the section
	for _, ctx := range contexts {
		chain2[0] = ctx
		for _, elem := range section.elems {
			if err := tmpl.renderElement(elem, chain2, buf); err != nil {
				return err
			}
		}
	}
	return nil
}

func JSONEscape(dest io.Writer, data string) error {
	for _, r := range data {
		var err error
		switch r {
		case '"', '\\':
			_, err = dest.Write([]byte("\\"))
			if err != nil {
				break
			}
			_, err = dest.Write([]byte(string(r)))
		case '\n':
			_, err = dest.Write([]byte(`\n`))
		case '\b':
			_, err = dest.Write([]byte(`\b`))
		case '\f':
			_, err = dest.Write([]byte(`\f`))
		case '\r':
			_, err = dest.Write([]byte(`\r`))
		case '\t':
			_, err = dest.Write([]byte(`\t`))
		default:
			if unicode.IsControl(r) {
				_, err = dest.Write([]byte(fmt.Sprintf("\\u%04x", r)))
			} else {
				_, err = dest.Write([]byte(string(r)))
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func getSectionText(elements []any, buf io.Writer) {
	for _, element := range elements {
		getElementText(element, buf)
	}
}

func getElementText(element any, buf io.Writer) {
	switch elem := element.(type) {
	case *textElement:
		fmt.Fprintf(buf, "%s", elem.text)
	case *varElement:
		fmt.Fprintf(buf, "{{%s}}", elem.name)
	case *sectionElement:
		if elem.inverted {
			fmt.Fprintf(buf, "{{^%s}}", elem.name)
		} else {
			fmt.Fprintf(buf, "{{#%s}}", elem.name)
		}
		for _, nelem := range elem.elems {
			getElementText(nelem, buf)
		}
		fmt.Fprintf(buf, "{{/%s}}", elem.name)
	case *Template:
		fmt.Fprint(buf, "???")
	}
}

func (tmpl *Template) renderElement(element any, contextChain []any, buf io.Writer) error {
	switch elem := element.(type) {
	case *textElement:
		_, err := buf.Write(elem.text)
		return err
	case *varElement:
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Panic while looking up %q: %s\n", elem.name, r)
			}
		}()
		val, err := lookup(contextChain, elem.name, tmpl.errorOnMissing)
		if err != nil {
			return err
		}

		if val.IsValid() {
			if elem.raw {
				fmt.Fprint(buf, val.Interface())
			} else {
				s := fmt.Sprint(val.Interface())
				switch tmpl.outputMode {
				case EscapeJSON:
					// Whether to use JSON's marshalling (true) or our JSON escaping (false)
					useMarshal := false

					var kind reflect.Kind
					typeof := reflect.TypeOf(val.Interface())
					if typeof != nil {
						kind = typeof.Kind()
					}

					// Output arrays and objects in JSON format, if in JSON mode
					if kind == reflect.Slice || kind == reflect.Array || kind == reflect.Map {
						useMarshal = true
					}

					//
					// Special case overrides
					//
					if typeof != nil {
						switch typeof.String() {
						case "uuid.UUID":
							// JSON's implementation encloses UUID in double-quotes,
							// so use ours instead
							useMarshal = false
						case "[]uint8":
							// JSON's implementation encloses the base64 encoded value in double quotes,
							// so use ours instead
							if ba, ok := val.Interface().([]byte); ok {
								s = string(ba[:])
								useMarshal = false
							}
						}
					}

					if useMarshal {
						marshalledJson, err := json.Marshal(val.Interface())
						if err != nil {
							return err
						}
						_, err = buf.Write(marshalledJson)
						if err != nil {
							return err
						}
						break
					}
					if err = JSONEscape(buf, s); err != nil {
						return err
					}
				case EscapeHTML:
					template.HTMLEscape(buf, []byte(s))
				case Raw:
					if _, err = buf.Write([]byte(s)); err != nil {
						return err
					}
				}
			}
		}
	case *sectionElement:
		if err := tmpl.renderSection(elem, contextChain, buf); err != nil {
			return err
		}
	case *partialElement:
		partial, err := tmpl.getPartials(elem.prov, elem.name, elem.indent)
		if err != nil {
			if tmpl.errorOnMissing {
				return err
			}
			return nil
		}
		if err := partial.renderTemplate(contextChain, buf); err != nil {
			return err
		}
	}
	return nil
}

func (tmpl *Template) renderTemplate(contextChain []any, buf io.Writer) error {
	for _, elem := range tmpl.elems {
		if err := tmpl.renderElement(elem, contextChain, buf); err != nil {
			return err
		}
	}
	return nil
}

// Frender uses the given data source - generally a map or struct - to
// render the compiled template to an io.Writer.
func (tmpl *Template) Frender(out io.Writer, context ...any) error {
	var contextChain []any
	for _, c := range context {
		val := reflect.ValueOf(c)
		contextChain = append(contextChain, val)
	}
	return tmpl.renderTemplate(contextChain, out)
}

// Render uses the given data source - generally a map or struct - to render
// the compiled template and return the output.
func (tmpl *Template) Render(context ...any) (string, error) {
	var buf bytes.Buffer
	err := tmpl.Frender(&buf, context...)
	return buf.String(), err
}

// RenderInLayout uses the given data source - generally a map or struct - to
// render the compiled template and layout "wrapper" template and return the
// output.
func (tmpl *Template) RenderInLayout(layout *Template, context ...any) (string, error) {
	var buf bytes.Buffer
	err := tmpl.FRenderInLayout(&buf, layout, context...)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FRenderInLayout uses the given data source - generally a map or
// struct - to render the compiled templated a loayout "wrapper"
// template to an io.Writer.
func (tmpl *Template) FRenderInLayout(out io.Writer, layout *Template, context ...any) error {
	content, err := tmpl.Render(context...)
	if err != nil {
		return err
	}
	allContext := make([]any, len(context)+1)
	copy(allContext[1:], context)
	allContext[0] = map[string]string{"content": content}
	return layout.Frender(out, allContext...)
}
