package mustache

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type Test struct {
	tmpl     string
	context  any
	expected string
	err      error
}

type Data struct {
	A bool
	B string
}

type User struct {
	Name string
	ID   int64
}

type Settings struct {
	Allow bool
}

func (u User) Func1() string {
	return u.Name
}

func (u *User) Func2() string {
	return u.Name
}

func (u *User) Func3() (map[string]string, error) {
	return map[string]string{"name": u.Name}, nil
}

func (u *User) Func4() (map[string]string, error) {
	return nil, nil
}

func (u *User) Func5() (*Settings, error) {
	return &Settings{true}, nil
}

func (u *User) Func6() ([]any, error) {
	var v []any
	v = append(v, &Settings{true})
	return v, nil
}

func (u User) Truefunc1() bool {
	return true
}

func (u *User) Truefunc2() bool {
	return true
}

func makeVector(n int) []any {
	var v []any
	for i := 0; i < n; i++ {
		v = append(v, &User{"Mike", 1})
	}
	return v
}

type Category struct {
	Tag         string
	Description string
}

func (c Category) DisplayName() string {
	return c.Tag + " - " + c.Description
}

func TestTagType(t *testing.T) {
	tt := Partial
	ts := tt.String()
	if ts != "Partial" {
		t.Errorf("got %s, expected Partial", ts)
	}
}

var tests = []Test{
	{`{{/}}`, nil, "", parseError{line: 1, message: "unmatched close tag"}},
	{`hello world`, nil, "hello world", nil},
	{`hello {{name}}`, map[string]string{"name": "world"}, "hello world", nil},
	{`{{var}}`, map[string]string{"var": "5 > 2"}, "5 &gt; 2", nil},
	{`{{{var}}}`, map[string]string{"var": "5 > 2"}, "5 > 2", nil},
	{`{{var}}`, map[string]string{"var": "& \" < >"}, "&amp; &#34; &lt; &gt;", nil},
	{`{{{var}}}`, map[string]string{"var": "& \" < >"}, "& \" < >", nil},
	{`{{a}}{{b}}{{c}}{{d}}`, map[string]string{"a": "a", "b": "b", "c": "c", "d": "d"}, "abcd", nil},
	{`0{{a}}1{{b}}23{{c}}456{{d}}89`, map[string]string{"a": "a", "b": "b", "c": "c", "d": "d"}, "0a1b23c456d89", nil},
	{`hello {{! comment }}world`, map[string]string{}, "hello world", nil},
	{`{{ a }}{{=<% %>=}}<%b %><%={{ }}=%>{{ c }}`, map[string]string{"a": "a", "b": "b", "c": "c"}, "abc", nil},
	{`{{ a }}{{= <% %> =}}<%b %><%= {{ }}=%>{{c}}`, map[string]string{"a": "a", "b": "b", "c": "c"}, "abc", nil},

	// section tests
	{`{{#A}}`, Data{true, "hello"}, "", parseError{line: 1, message: "Section A has no closing tag"}},
	{`{{#A}}{{B}}{{/A}}`, Data{true, "hello"}, "hello", nil},
	{`{{#A}}{{{B}}}{{/A}}`, Data{true, "5 > 2"}, "5 > 2", nil},
	{`{{#A}}{{B}}{{/A}}`, Data{true, "5 > 2"}, "5 &gt; 2", nil},
	{`{{#A}}{{B}}{{/A}}`, Data{false, "hello"}, "", nil},
	{`{{a}}{{#b}}{{b}}{{/b}}{{c}}`, map[string]string{"a": "a", "b": "b", "c": "c"}, "abc", nil},
	{
		`{{#A}}{{B}}{{/A}}`,
		struct {
			A []struct {
				B string
			}
		}{[]struct {
			B string
		}{{"a"}, {"b"}, {"c"}}},
		"abc",
		nil,
	},
	{`{{#A}}{{b}}{{/A}}`, struct{ A []map[string]string }{[]map[string]string{{"b": "a"}, {"b": "b"}, {"b": "c"}}}, "abc", nil},

	{`{{#users}}{{Name}}{{/users}}`, map[string]any{"users": []User{{"Mike", 1}}}, "Mike", nil},

	{`{{#users}}gone{{Name}}{{/users}}`, map[string]any{"users": nil}, "", nil},
	{`{{#users}}gone{{Name}}{{/users}}`, map[string]any{"users": (*User)(nil)}, "", nil},
	{`{{#users}}gone{{Name}}{{/users}}`, map[string]any{"users": []User{}}, "", nil},

	{`{{#users}}{{Name}}{{/users}}`, map[string]any{"users": []*User{{"Mike", 1}}}, "Mike", nil},
	{`{{#users}}{{Name}}{{/users}}`, map[string]any{"users": []any{&User{"Mike", 12}}}, "Mike", nil},
	{`{{#users}}{{Name}}{{/users}}`, map[string]any{"users": makeVector(1)}, "Mike", nil},
	{`{{Name}}`, User{"Mike", 1}, "Mike", nil},
	{`{{Name}}`, &User{"Mike", 1}, "Mike", nil},
	{"{{#users}}\n{{Name}}\n{{/users}}", map[string]any{"users": makeVector(2)}, "Mike\nMike\n", nil},
	{"{{#users}}\r\n{{Name}}\r\n{{/users}}", map[string]any{"users": makeVector(2)}, "Mike\r\nMike\r\n", nil},
	// section with meta
	{`{{#a}}{{=<% %>=}}<p><% content %></p><%={{ }}=%>{{/a}}`, map[string]map[string]string{"a": {"content": "Content content"}}, "<p>Content content</p>", nil},

	// falsy: golang zero values
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": nil}, "", nil},
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": false}, "", nil},
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": 0}, "", nil},
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": 0.0}, "", nil},
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": ""}, "", nil},
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": Data{}}, "", nil},
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": []any{}}, "", nil},
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": [0]any{}}, "", nil},
	// falsy: special cases we disagree with golang
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": "\t"}, "", nil},
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": []any{0}}, "Hi 0", nil},
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": [1]any{0}}, "Hi 0", nil},

	// non-false sections have their value at the top of the context
	{"{{#a}}Hi {{.}}{{/a}}", map[string]any{"a": "Rob"}, "Hi Rob", nil},

	// section does not exist
	{`{{#has}}{{/has}}`, &User{"Mike", 1}, "", nil},

	// implicit iterator tests
	{`"{{#list}}({{.}}){{/list}}"`, map[string]any{"list": []string{"a", "b", "c", "d", "e"}}, "\"(a)(b)(c)(d)(e)\"", nil},
	{`"{{#list}}({{.}}){{/list}}"`, map[string]any{"list": []int{1, 2, 3, 4, 5}}, "\"(1)(2)(3)(4)(5)\"", nil},
	{`"{{#list}}({{.}}){{/list}}"`, map[string]any{"list": []float64{1.10, 2.20, 3.30, 4.40, 5.50}}, "\"(1.1)(2.2)(3.3)(4.4)(5.5)\"", nil},

	// inverted section tests
	{`{{a}}{{^b}}b{{/b}}{{c}}`, map[string]any{"a": "a", "b": false, "c": "c"}, "abc", nil},
	{`{{^a}}b{{/a}}`, map[string]any{"a": false}, "b", nil},
	{`{{^a}}b{{/a}}`, map[string]any{"a": true}, "", nil},
	{`{{^a}}b{{/a}}`, map[string]any{"a": "nonempty string"}, "", nil},
	{`{{^a}}b{{/a}}`, map[string]any{"a": []string{}}, "b", nil},
	{`{{a}}{{^b}}b{{/b}}{{c}}`, map[string]string{"a": "a", "c": "c"}, "abc", nil},

	// function tests
	{`{{#users}}{{Func1}}{{/users}}`, map[string]any{"users": []User{{"Mike", 1}}}, "Mike", nil},
	{`{{#users}}{{Func1}}{{/users}}`, map[string]any{"users": []*User{{"Mike", 1}}}, "Mike", nil},
	{`{{#users}}{{Func2}}{{/users}}`, map[string]any{"users": []*User{{"Mike", 1}}}, "Mike", nil},

	{`{{#users}}{{#Func3}}{{name}}{{/Func3}}{{/users}}`, map[string]any{"users": []*User{{"Mike", 1}}}, "Mike", nil},
	{`{{#users}}{{#Func4}}{{name}}{{/Func4}}{{/users}}`, map[string]any{"users": []*User{{"Mike", 1}}}, "", nil},
	{`{{#Truefunc1}}abcd{{/Truefunc1}}`, User{"Mike", 1}, "abcd", nil},
	{`{{#Truefunc1}}abcd{{/Truefunc1}}`, &User{"Mike", 1}, "abcd", nil},
	{`{{#Truefunc2}}abcd{{/Truefunc2}}`, &User{"Mike", 1}, "abcd", nil},
	{`{{#Func5}}{{#Allow}}abcd{{/Allow}}{{/Func5}}`, &User{"Mike", 1}, "abcd", nil},
	{`{{#user}}{{#Func5}}{{#Allow}}abcd{{/Allow}}{{/Func5}}{{/user}}`, map[string]any{"user": &User{"Mike", 1}}, "abcd", nil},
	{`{{#user}}{{#Func6}}{{#Allow}}abcd{{/Allow}}{{/Func6}}{{/user}}`, map[string]any{"user": &User{"Mike", 1}}, "abcd", nil},

	// context chaining
	{`hello {{#section}}{{name}}{{/section}}`, map[string]any{"section": map[string]string{"name": "world"}}, "hello world", nil},
	{`hello {{#section}}{{name}}{{/section}}`, map[string]any{"name": "bob", "section": map[string]string{"name": "world"}}, "hello world", nil},
	{`hello {{#bool}}{{#section}}{{name}}{{/section}}{{/bool}}`, map[string]any{"bool": true, "section": map[string]string{"name": "world"}}, "hello world", nil},
	{`{{#users}}{{canvas}}{{/users}}`, map[string]any{"canvas": "hello", "users": []User{{"Mike", 1}}}, "hello", nil},
	{`{{#categories}}{{DisplayName}}{{/categories}}`, map[string][]*Category{
		"categories": {&Category{"a", "b"}},
	}, "a - b", nil},

	{
		`{{#section}}{{#bool}}{{x}}{{/bool}}{{/section}}`,
		map[string]any{
			"x": "broken",
			"section": []map[string]any{
				{"x": "working", "bool": true},
				{"x": "nope", "bool": false},
			},
		},
		"working", nil,
	},

	{
		`{{#section}}{{^bool}}{{x}}{{/bool}}{{/section}}`,
		map[string]any{
			"x": "broken",
			"section": []map[string]any{
				{"x": "working", "bool": false},
				{"x": "nope", "bool": true},
			},
		},
		"working", nil,
	},

	// dotted names(dot notation)
	{`"{{person.name}}" == "{{#person}}{{name}}{{/person}}"`, map[string]any{"person": map[string]string{"name": "Joe"}}, `"Joe" == "Joe"`, nil},
	{`"{{{person.name}}}" == "{{#person}}{{{name}}}{{/person}}"`, map[string]any{"person": map[string]string{"name": "Joe"}}, `"Joe" == "Joe"`, nil},
	{`"{{a.b.c.d.e.name}}" == "Phil"`, map[string]any{"a": map[string]any{"b": map[string]any{"c": map[string]any{"d": map[string]any{"e": map[string]string{"name": "Phil"}}}}}}, `"Phil" == "Phil"`, nil},
	{`"{{#a}}{{b.c.d.e.name}}{{/a}}" == "Phil"`, map[string]any{"a": map[string]any{"b": map[string]any{"c": map[string]any{"d": map[string]any{"e": map[string]string{"name": "Phil"}}}}}, "b": map[string]any{"c": map[string]any{"d": map[string]any{"e": map[string]string{"name": "Wrong"}}}}}, `"Phil" == "Phil"`, nil},
}

func TestBasic(t *testing.T) {
	// Default behavior, AllowMissingVariables=true
	for _, test := range tests {
		tm, err := New().CompileString(test.tmpl)
		var output string
		if err == nil && tm != nil {
			output, err = tm.Render(test.tmpl, test.context)
		}
		if err != test.err {
			t.Errorf("%q expected %q but got error %v", test.tmpl, test.expected, err)
		} else if output != test.expected {
			t.Errorf("%q expected %q got %q", test.tmpl, test.expected, output)
		}
	}
	/*
		for _, test := range tests {
			tm, err := ParseString(test.tmpl)
			var output string
			if err == nil && tm != nil {
				tm.errorOnMissing = true
				output, err = tm.Render(test.tmpl, test.context)
			}
			if err != test.err {
				t.Errorf("%s expected %s but got error %s", test.tmpl, test.expected, err.Error())
			} else if output != test.expected {
				t.Errorf("%q expected %q got %q", test.tmpl, test.expected, output)
			}
		}

	*/
}

var missing = []Test{
	// does not exist
	{`{{dne}}`, map[string]string{"name": "world"}, "", nil},
	{`{{dne}}`, User{"Mike", 1}, "", nil},
	{`{{dne}}`, &User{"Mike", 1}, "", nil},
	// dotted names(dot notation)
	{`"{{a.b.c}}" == ""`, map[string]any{}, `"" == ""`, nil},
	{`"{{a.b.c.name}}" == ""`, map[string]any{"a": map[string]any{"b": map[string]string{}}, "c": map[string]string{"name": "Jim"}}, `"" == ""`, nil},
	{`{{#a}}{{b.c}}{{/a}}`, map[string]any{"a": map[string]any{"b": map[string]string{}}, "b": map[string]string{"c": "ERROR"}}, "", nil},
}

func TestMissing(t *testing.T) {
	// Default behavior, AllowMissingVariables=true
	for _, test := range missing {
		cmpl, err := New().CompileString(test.tmpl)
		if err != nil {
			t.Error(err)
		}
		output, err := cmpl.Render(test.context)
		if err != nil {
			t.Error(err)
		}
		if output != test.expected {
			t.Errorf("%q expected %q got %q", test.tmpl, test.expected, output)
		}
	}

	// Now set AllowMissingVariables=false and confirm we get errors.
	for _, test := range missing {
		tm, err := New().WithErrors(true).CompileString(test.tmpl)
		if err != nil {
			t.Error(err)
		}
		output, err := tm.Render(test.tmpl, test.context)
		if err == nil {
			t.Errorf("%q expected missing variable error but got %q", test.tmpl, output)
		} else if !strings.Contains(err.Error(), "missing variable") {
			t.Errorf("%q expected missing variable error but got %q", test.tmpl, err.Error())
		}
	}
}

func TestFile(t *testing.T) {
	filename := path.Join(path.Join(os.Getenv("PWD"), "tests"), "test1.mustache")
	expected := "hello world"
	cmpl, err := New().CompileFile(filename)
	if err != nil {
		t.Error(err)
	}
	output, err := cmpl.Render(map[string]string{"name": "world"})
	if err != nil {
		t.Error(err)
	} else if output != expected {
		t.Errorf("testfile expected %q got %q", expected, output)
	}
}

func TestFRender(t *testing.T) {
	filename := path.Join(path.Join(os.Getenv("PWD"), "tests"), "test1.mustache")
	expected := "hello world"
	tmpl, err := New().CompileFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err = tmpl.Frender(&buf, map[string]string{"name": "world"})
	if err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if output != expected {
		t.Fatalf("testfile expected %q got %q", expected, output)
	}
}

func TestPartial(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Error(err)
	}
	testdir := path.Join(cwd, "tests")
	filename := path.Join(testdir, "test2.mustache")
	expected := "hello world"
	tmpl, err := New().WithErrors(true).
		WithPartials(&FileProvider{Paths: []string{testdir}, Extensions: []string{".mustache"}}).
		CompileFile(filename)
	if err != nil {
		t.Error(err)
		return
	}
	output, err := tmpl.Render(map[string]string{"Name": "world"})
	if err != nil {
		t.Error(err)
		return
	} else if output != expected {
		t.Errorf("testpartial expected %q got %q", expected, output)
		return
	}

	expectedTags := []tag{
		{
			Type: Partial,
			Name: "partial",
		},
	}
	compareTags(t, tmpl.Tags(), expectedTags)
}

func TestPartialSafety(t *testing.T) {
	tmpl, err := New().WithErrors(true).WithPartials(&FileProvider{}).CompileString("{{>../unsafe}}")
	if err != nil {
		t.Error(err)
	}
	txt, err := tmpl.Render(nil)
	if err == nil {
		t.Errorf("expected error for unsafe partial")
	}
	if txt != "" {
		t.Errorf("expected unsafe partial to fail")
	}
}

func TestPartialSafetyWindows(t *testing.T) {
	tmpl, err := New().WithErrors(true).WithPartials(&FileProvider{}).CompileString("{{>spec/..\\..\\test.txt}}")
	if err != nil {
		t.Error(err)
	}
	txt, err := tmpl.Render(nil)
	if err == nil {
		t.Errorf("expected error for unsafe partial")
	}
	if txt != "" {
		t.Errorf("expected unsafe partial to fail")
	}
}

func TestJSONEscape(t *testing.T) {
	tests := []struct {
		Before string
		After  string
	}{
		{`'single quotes'`, `'single quotes'`},
		{`"double quotes"`, `\"double quotes\"`},
		{`\backslash\`, `\\backslash\\`},
		{"some\tcontrol\ncharacters\x1c\b\f\r", `some\tcontrol\ncharacters\u001c\b\f\r`},
		{`🦜`, `🦜`},
	}
	var buf bytes.Buffer
	for _, tst := range tests {
		if err := JSONEscape(&buf, tst.Before); err != nil {
			t.Error(err)
		}
		txt := buf.String()
		if txt != tst.After {
			t.Errorf("got %s expected %s", txt, tst.After)
		}
		buf.Reset()
	}
}

func TestRenderRaw(t *testing.T) {
	tests := []struct {
		Template string
		Data     map[string]any
		Result   string
	}{
		{
			Template: `{{a}} {{b}} {{c}}`,
			Data:     map[string]any{"a": `<a href="">`, "b": "}o&o{", "c": "\t"},
			Result:   "<a href=\"\"> }o&o{ \t",
		},
	}
	for _, tst := range tests {
		tmpl, err := New().WithEscapeMode(Raw).CompileString(tst.Template)
		if err != nil {
			t.Error(err)
		}
		txt, err := tmpl.Render(tst.Data)
		if err != nil {
			t.Error(err)
		}
		if txt != tst.Result {
			t.Errorf("expected %s got %s", tst.Result, txt)
		}
	}
}

func TestRenderJSON(t *testing.T) {
	type item struct {
		Emoji string
		Name  string
	}

	testUuid := uuid.New()

	tests := []struct {
		Template string
		Data     map[string]any
		Result   string
	}{
		{
			Template: `{"a": "{{a}}", "b": "{{b}}", "c": "{{c}}"}`,
			Data:     map[string]any{"a": "Text\nwith\tcontrols", "b": `"I said 'No!'"`, "c": "EOF\u001cHERE"},
			Result:   `{"a": "Text\nwith\tcontrols", "b": "\"I said 'No!'\"", "c": "EOF\u001cHERE"}`,
		},
		{
			Template: `{"a": [""{{#a}},"{{.}}"{{/a}}]}`,
			Data:     map[string]any{"a": []int{1, 2, 3}},
			Result:   `{"a": ["","1","2","3"]}`,
		},
		{
			Template: `"{{#values}}{{Emoji}}{{Name}} {{/values}}"`,
			Data: map[string]any{
				"values": any([]item{
					{
						Emoji: "🟡",
						Name:  "Rico",
					},
					{
						Emoji: "🟢",
						Name:  "Bruce",
					},
					{
						Emoji: "🔵",
						Name:  "Luna",
					},
				}),
			},
			Result: `"🟡Rico 🟢Bruce 🔵Luna "`,
		},
		{
			Template: `{{object}}`,
			Data: map[string]any{
				"object": map[string]any{
					"a": "alpha", "b": "beta",
				},
			},
			Result: `{"a":"alpha","b":"beta"}`,
		},
		{
			Template: `{{array}}`,
			Data: map[string]any{
				"array": []int{
					4, 5, 6,
				},
			},
			Result: `[4,5,6]`,
		},
		{
			Template: `{{uuid}}`,
			Data: map[string]any{
				"uuid": testUuid,
			},
			Result: testUuid.String(),
		},
		{
			Template: `{{byteAry}}`,
			Data: map[string]any{
				"byteAry": []byte("foobar🟡"),
			},
			Result: "foobar🟡",
		},
	}
	for _, tst := range tests {
		tmpl, err := New().WithEscapeMode(EscapeJSON).CompileString(tst.Template)
		if err != nil {
			t.Error(err)
		}
		tmpl.outputMode = EscapeJSON
		txt, err := tmpl.Render(tst.Data)
		if err != nil {
			t.Error(err)
		}
		if txt != tst.Result {
			t.Errorf("expected %s got %s", tst.Result, txt)
		}
	}
}

// Make sure bugs caught by fuzz testing don't creep back in
func TestCrashers(t *testing.T) {
	crashers := []string{
		`{{#}}{{#}}{{#}}{{#}}{{#}}{{=}}`,
		`{{#}}{{#}}{{#}}{{#}}{{#}}{{#}}{{#}}{{#}}{{=}}`,
		`{{=}}`,
	}
	for _, c := range crashers {
		_, err := New().CompileString(c)
		if err == nil {
			t.Error(err)
		}
	}
}

/*
	func TestSectionPartial(t *testing.T) {
	    filename := path.Join(path.Join(os.Getenv("PWD"), "tests"), "test3.mustache")
	    expected := "Mike\nJoe\n"
	    context := map[string]any{"users": []User{{"Mike", 1}, {"Joe", 2}}}
	    output := RenderFile(filename, context)
	    if output != expected {
	        t.Fatalf("testSectionPartial expected %q got %q", expected, output)
	    }
	}
*/
func TestMultiContext(t *testing.T) {
	tmpl, err := New().CompileString(`{{hello}} {{World}}`)
	if err != nil {
		t.Error(err)
	}
	output, err := tmpl.Render(map[string]string{"hello": "hello"}, struct{ World string }{"world"})
	if err != nil {
		t.Error(err)
	}
	tmpl2, err := New().CompileString(`{{hello}} {{World}}`)
	if err != nil {
		t.Error(err)
	}
	output2, err := tmpl2.Render(struct{ World string }{"world"}, map[string]string{"hello": "hello"})
	if err != nil {
		t.Error(err)
	}
	if output != "hello world" || output2 != "hello world" {
		t.Errorf("TestMultiContext expected %q got %q", "hello world", output)
	}
}

func lambda(text string, render RenderFn, res string, data map[string]any) (string, error) {
	d, err := render(text)
	data[res] = d
	if err == nil {
		return "OK", nil
	}
	return "", err
}

func TestLambda(t *testing.T) {
	templ := `Call:{{#lambda}}hello {{lookup}} {{#sub}}{{.}} {{/sub}}{{^negsub}}nothing{{/negsub}}{{/lambda}};Result:{{result}}`
	data := make(map[string]any)
	data["lookup"] = "world"
	data["sub"] = []string{"subv1", "subv2"}
	data["negsub"] = nil
	data["lambda"] = func(text string, render RenderFn) (string, error) {
		return lambda(text, render, "result", data)
	}
	tmpl, err := New().CompileString(templ)
	if err != nil {
		t.Error(err)
	}
	output, _ := tmpl.Render(templ, data)
	expect := "Call:OK;Result:hello world subv1 subv2 nothing"
	if output != expect {
		t.Fatalf("TestMultiContext expected %q got %q", expect, output)
	}
}

func TestLambdaError(t *testing.T) {
	templ := `stop_at_error.{{#lambda}}{{/lambda}}.never_here`
	data := make(map[string]any)
	data["lambda"] = func(text string, render RenderFn) (string, error) {
		return "", fmt.Errorf("test err")
	}
	tmpl, err := New().CompileString(templ)
	if err != nil {
		t.Error(err)
	}
	output, _ := tmpl.Render(data)
	expect := "stop_at_error."
	if output != expect {
		t.Fatalf("TestLambdaError expected %q got %q", expect, output)
	}
}

var malformed = []Test{
	{`{{#a}}{{}}{{/a}}`, Data{true, "hello"}, "", fmt.Errorf("line 1: empty tag")},
	{`{{}}`, nil, "", fmt.Errorf("line 1: empty tag")},
	{`{{}`, nil, "", fmt.Errorf("line 1: unmatched open tag")},
	{`{{`, nil, "", fmt.Errorf("line 1: unmatched open tag")},
	// invalid syntax - https://github.com/hoisie/mustache/issues/10
	{`{{#a}}{{#b}}{{/a}}{{/b}}}`, map[string]any{}, "", fmt.Errorf("line 1: interleaved closing tag: a")},
}

func TestMalformed(t *testing.T) {
	for _, test := range malformed {
		tmpl, err := New().CompileString(test.tmpl)
		var output string
		if err == nil {
			output, err = tmpl.Render(test.context)
		}
		if err != nil {
			if test.err == nil {
				t.Error(err)
			} else if test.err.Error() != err.Error() {
				t.Errorf("%q expected error %q but got error %q", test.tmpl, test.err.Error(), err.Error())
			}
		} else {
			if test.err == nil {
				t.Errorf("%q expected %q got %q", test.tmpl, test.expected, output)
			} else {
				t.Errorf("%q expected error %q but got %q", test.tmpl, test.err.Error(), output)
			}
		}
	}
}

type LayoutTest struct {
	layout   string
	tmpl     string
	context  any
	expected string
}

var layoutTests = []LayoutTest{
	{`Header {{content}} Footer`, `Hello World`, nil, `Header Hello World Footer`},
	{`Header {{content}} Footer`, `Hello {{s}}`, map[string]string{"s": "World"}, `Header Hello World Footer`},
	{`Header {{content}} Footer`, `Hello {{content}}`, map[string]string{"content": "World"}, `Header Hello World Footer`},
	{`Header {{extra}} {{content}} Footer`, `Hello {{content}}`, map[string]string{"content": "World", "extra": "extra"}, `Header extra Hello World Footer`},
	{`Header {{content}} {{content}} Footer`, `Hello {{content}}`, map[string]string{"content": "World"}, `Header Hello World Hello World Footer`},
}

func TestLayout(t *testing.T) {
	for _, test := range layoutTests {
		tmpl, err := New().CompileString(test.tmpl)
		if err != nil {
			t.Error(err)
		}
		tmpl2, err := New().CompileString(test.layout)
		if err != nil {
			t.Error(err)
		}
		output, err := tmpl.RenderInLayout(tmpl2, test.context)
		if err != nil {
			t.Error(err)
		} else if output != test.expected {
			t.Errorf("%q expected %q got %q", test.tmpl, test.expected, output)
		}
	}
}

func TestLayoutToWriter(t *testing.T) {
	for _, test := range layoutTests {
		tmpl, err := New().CompileString(test.tmpl)
		if err != nil {
			t.Error(err)
			continue
		}
		layoutTmpl, err := New().CompileString(test.layout)
		if err != nil {
			t.Error(err)
			continue
		}
		var buf bytes.Buffer
		err = tmpl.FRenderInLayout(&buf, layoutTmpl, test.context)
		if err != nil {
			t.Error(err)
		} else if buf.String() != test.expected {
			t.Errorf("%q expected %q got %q", test.tmpl, test.expected, buf.String())
		}
	}
}

type Person struct {
	FirstName string
	LastName  string
}

func (p *Person) Name1() string {
	return p.FirstName + " " + p.LastName
}

func (p Person) Name2() string {
	return p.FirstName + " " + p.LastName
}

func TestPointerReceiver(t *testing.T) {
	p := Person{"John", "Smith"}
	tests := []struct {
		tmpl     string
		context  any
		expected string
	}{
		{
			tmpl:     "{{Name1}}",
			context:  &p,
			expected: "John Smith",
		},
		{
			tmpl:     "{{Name2}}",
			context:  &p,
			expected: "John Smith",
		},
		{
			tmpl:     "{{Name1}}",
			context:  p,
			expected: "",
		},
		{
			tmpl:     "{{Name2}}",
			context:  p,
			expected: "John Smith",
		},
	}
	for _, test := range tests {
		tmpl, err := New().CompileString(test.tmpl)
		if err != nil {
			t.Error(err)
		}
		output, err := tmpl.Render(test.context)
		if err != nil {
			t.Error(err)
		} else if output != test.expected {
			t.Errorf("expected %q got %q", test.expected, output)
		}
	}
}

type tag struct {
	Type TagType
	Name string
	Tags []tag
}

type tagsTest struct {
	tmpl string
	tags []tag
}

var tagTests = []tagsTest{
	{
		tmpl: `hello world`,
		tags: nil,
	},
	{
		tmpl: `hello {{name}}`,
		tags: []tag{
			{
				Type: Variable,
				Name: "name",
			},
		},
	},
	{
		tmpl: `{{#name}}hello {{name}}{{/name}}{{^name}}hello {{name2}}{{/name}}`,
		tags: []tag{
			{
				Type: Section,
				Name: "name",
				Tags: []tag{
					{
						Type: Variable,
						Name: "name",
					},
				},
			},
			{
				Type: InvertedSection,
				Name: "name",
				Tags: []tag{
					{
						Type: Variable,
						Name: "name2",
					},
				},
			},
		},
	},
}

func TestTags(t *testing.T) {
	for _, test := range tagTests {
		testTags(t, &test)
	}
}

func testTags(t *testing.T, test *tagsTest) {
	tmpl, err := New().CompileString(test.tmpl)
	if err != nil {
		t.Error(err)
		return
	}
	compareTags(t, tmpl.Tags(), test.tags)
}

func compareTags(t *testing.T, actual []Tag, expected []tag) {
	if len(actual) != len(expected) {
		t.Errorf("expected %d tags, got %d", len(expected), len(actual))
		return
	}
	for i, tag := range actual {
		if tag.Type() != expected[i].Type {
			t.Errorf("expected %s, got %s", expected[i].Type, tag.Type())
			return
		}
		if tag.Name() != expected[i].Name {
			t.Errorf("expected %s, got %s", expected[i].Name, tag.Name())
			return
		}

		switch tag.Type() {
		case Variable:
			if len(expected[i].Tags) != 0 {
				t.Errorf("expected %d tags, got 0", len(expected[i].Tags))
				return
			}
		case Section, InvertedSection:
			compareTags(t, tag.Tags(), expected[i].Tags)
		case Partial:
			compareTags(t, tag.Tags(), expected[i].Tags)
		case Invalid:
			t.Errorf("invalid tag type: %s", tag.Type())
			return
		default:
			t.Errorf("invalid tag type: %s", tag.Type())
			return
		}
	}
}
