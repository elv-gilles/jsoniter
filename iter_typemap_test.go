package jsoniter

import (
	"github.com/modern-go/reflect2"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

func TestGetInterface(t *testing.T) {
	var iType = reflect.TypeOf((*Intf)(nil))
	intfType := reflect2.ConfigUnsafe.Type2(iType.Elem())

	s := "jsoniter.ImplA"
	impltyp := reflect2.TypeByName(s)
	require.NotNil(t, impltyp)

	//var intf Intf
	//intf = &ImplA{}
	//ty := reflect2.TypeOf(intf)
	//fmt.Println("kind", iType.Kind(), iType.Elem(), iType.Elem().Kind())
	//fmt.Println("type of instance", ty.String(), "impl intf", reflect.TypeOf(intf).Implements(iType.Elem()))

	// cannot get interface by its name
	intfName := "jsoniter.Intf"
	_ = reflect2.TypeByName(intfName)
	//require.NotNil(t, intftyp)
	require.Equal(t, intfName, intfType.String())

	a := reflect2.TypeByName("jsoniter.and")
	require.NotNil(t, a)
}

func TestTypeMappingNoDecode(t *testing.T) {
	enc := (&Config{
		SortMapKeys: true,
	}).Froze()

	c1 := &Cplx{Intf: NewA("abcd")}
	js1, err := enc.Marshal(c1)
	require.NoError(t, err)

	cfg := &Config{
		SortMapKeys: false,
		TagKey:      "json",
	}
	api := cfg.Froze()

	var iType = reflect.TypeOf((*Intf)(nil))
	intfType := reflect2.ConfigUnsafe.Type2(iType.Elem())
	noop := &noopExtension{
		DummyExtension: DummyExtension{},
		target:         intfType,
	}
	api.RegisterExtension(noop)

	dc1 := &Cplx{}
	err = api.Unmarshal(js1, dc1)
	require.Error(t, err)
	require.True(t, noop.gotIt)
}

type noopExtension struct {
	DummyExtension
	target reflect2.Type
	gotIt  bool
}

func (e *noopExtension) CreateDecoder(typ reflect2.Type) ValDecoder {
	if e.target == typ {
		e.gotIt = true
	}
	return nil
}

func TestTypeMappingDecode(t *testing.T) {
	enc := (&Config{
		SortMapKeys: true,
	}).Froze()

	c1 := &Cplx{Intf: NewA("abcd")}
	js1, err := enc.Marshal(c1)
	require.NoError(t, err)

	c2 := &Cplx{Intf: NewB("1234")}
	js2, err := enc.Marshal(c2)
	require.NoError(t, err)

	cfg := &Config{
		SortMapKeys: false,
		TagKey:      "json",
	}
	api := cfg.Froze()

	typMap := &TypeMapping{
		Interface: reflect.TypeOf((*Intf)(nil)),
		TypeField: "type",
		SubTypes: map[string]string{
			"ImplA": "jsoniter.ImplA",
			"ImplB": "jsoniter.ImplB",
		},
	}

	ext, err := NewTypeMappingExt(typMap)
	require.NoError(t, err)

	api.RegisterExtension(ext)

	dc1 := &Cplx{}
	err = api.Unmarshal(js1, dc1)
	require.NoError(t, err)
	require.Equal(t, c1, dc1)

	dc2 := &Cplx{}
	err = api.Unmarshal(js2, dc2)
	require.NoError(t, err)
	require.Equal(t, c2, dc2)

	up1 := &Upper{
		Cplx: c1,
	}
	bjsu, err := enc.Marshal(up1)
	require.NoError(t, err)

	dup := &Upper{}
	err = api.Unmarshal(bjsu, dup)
	require.NoError(t, err)
	require.Equal(t, c1, dup.Cplx)

}

func TestTypeMappingDecodeErrorTypeFieldInvalid(t *testing.T) {

	for _, s1 := range []string{
		`{"intf":{"type":"ImplA,"a":"abcd"`,
		`{"intf":{"type":"ImplA,"a":"abcd"}}`,
		`{"intf":{"type":"Bla","a":"abcd"}}`,
		`{"intf":{"a":"abcd"}}`,
		`{"intf":{"type":{"name": "john", "address": "here"},"a":"abcd"}}`,
		`{"intf":{"type":0,"a":"abcd"}}`,
	} {
		cfg := &Config{
			SortMapKeys: false,
			TagKey:      "json",
		}
		api := cfg.Froze()
		typMap := &TypeMapping{
			Interface: reflect.TypeOf((*Intf)(nil)),
			TypeField: "type",
			SubTypes: map[string]string{
				"ImplA": "jsoniter.ImplA",
				"ImplB": "jsoniter.ImplB",
			},
		}
		ext, err := NewTypeMappingExt(typMap)
		require.NoError(t, err)
		api.RegisterExtension(ext)

		dc1 := &Cplx{}
		err = api.Unmarshal([]byte(s1), dc1)
		require.Error(t, err)
		//fmt.Println(err)
	}
}

func TestTypeMappingDecodeMultiple(t *testing.T) {
	type testCase struct {
		m1 *Multi
	}
	for _, tcase := range []*testCase{
		{
			m1: &Multi{
				Right: NewA("abcd"),
				Left:  NewB("1234"),
			},
		},
		{
			m1: &Multi{
				Left:  NewA("abcd"),
				Right: NewB("1234"),
			},
		},
	} {
		enc := (&Config{
			SortMapKeys: true,
		}).Froze()

		js1, err := enc.Marshal(tcase.m1)
		require.NoError(t, err)
		//fmt.Println(string(js1))

		// decode
		cfg := &Config{
			SortMapKeys: false,
			TagKey:      "json",
		}
		api := cfg.Froze()
		{
			// add extension
			typeMap := &TypeMapping{
				Interface: reflect.TypeOf((*Intf)(nil)),
				TypeField: "type",
				SubTypes: map[string]string{
					"ImplA": "jsoniter.ImplA",
					"ImplB": "jsoniter.ImplB",
				},
			}
			ext, err := NewTypeMappingExt(typeMap)
			require.NoError(t, err)
			api.RegisterExtension(ext)
		}

		dc1 := &Multi{}
		err = api.Unmarshal(js1, dc1)
		require.NoError(t, err)
		require.Equal(t, tcase.m1, dc1)
	}
}

func TestTypeMappingDecodeNested(t *testing.T) {
	nested := newNested("xyz")
	nested.C = &Cplx{Intf: NewA("abcd")}
	nested.D = NewB("1234")
	n1 := &NestedOwner{
		Nested: nested,
	}
	enc := (&Config{
		SortMapKeys: true,
	}).Froze()
	js1, err := enc.Marshal(n1)
	require.NoError(t, err)
	//fmt.Println(string(js1))

	cfg := &Config{
		SortMapKeys: false,
		TagKey:      "json",
	}
	api := cfg.Froze()
	// add extension
	typeMap := &TypeMapping{
		Interface: reflect.TypeOf((*Intf)(nil)),
		TypeField: "type",
		SubTypes: map[string]string{
			"ImplA":  "jsoniter.ImplA",
			"ImplB":  "jsoniter.ImplB",
			"Nested": "jsoniter.Nested",
		},
	}
	ext, err := NewTypeMappingExt(typeMap)
	require.NoError(t, err)
	api.RegisterExtension(ext)

	dc1 := &NestedOwner{}
	err = api.Unmarshal(js1, dc1)
	require.NoError(t, err)

	js2, err := enc.Marshal(dc1)
	require.NoError(t, err)
	_ = js2
	//fmt.Println(string(js1))
	//fmt.Println(string(js2))

	require.Equal(t, n1, dc1)
}

func TestTypeMappingDecodeRecursive(t *testing.T) {
	eval := &Recursor{
		Expr: nsub1(nsub1(nsub2(nil))),
	}
	enc := (&Config{
		SortMapKeys: true,
	}).Froze()
	js1, err := enc.Marshal(eval)
	require.NoError(t, err)
	//fmt.Println(string(js1))

	cfg := &Config{
		SortMapKeys: false,
		TagKey:      "json",
	}
	api := cfg.Froze()
	// add extension
	typeMap := &TypeMapping{
		Interface: reflect.TypeOf((*expr)(nil)),
		TypeField: "type",
		SubTypes: map[string]string{
			"sub1": "jsoniter.Sub1",
			"sub2": "jsoniter.Sub2",
		},
	}
	ext, err := NewTypeMappingExt(typeMap)
	require.NoError(t, err)
	api.RegisterExtension(ext)

	dc1 := &Recursor{}
	err = api.Unmarshal(js1, dc1)
	require.NoError(t, err)

	js2, err := enc.Marshal(dc1)
	require.NoError(t, err)
	_ = js2
	//fmt.Println(string(js1))
	//fmt.Println(string(js2))

	require.Equal(t, eval, dc1)
}

func TestTypeMappingDecodeSlice(t *testing.T) {
	eval := &evaluator{
		Expr: nor([]expr{
			nand([]expr{
				nboolean(true),
				nboolean(false),
			}),
			nand([]expr{
				nboolean(true),
				nboolean(true),
			}),
		}),
	}
	enc := (&Config{
		SortMapKeys: true,
	}).Froze()
	js1, err := enc.Marshal(eval)
	require.NoError(t, err)
	//fmt.Println(string(js1))

	cfg := &Config{
		SortMapKeys: false,
		TagKey:      "json",
	}
	api := cfg.Froze()
	// add extension
	typeMap := &TypeMapping{
		Interface: reflect.TypeOf((*expr)(nil)),
		TypeField: "type",
		SubTypes: map[string]string{
			"and":  "jsoniter.and",
			"or":   "jsoniter.or",
			"bool": "jsoniter.boolean",
		},
	}
	ext, err := NewTypeMappingExt(typeMap)
	require.NoError(t, err)
	api.RegisterExtension(ext)

	dc1 := &evaluator{}
	err = api.Unmarshal(js1, dc1)
	require.NoError(t, err)

	js2, err := enc.Marshal(dc1)
	require.NoError(t, err)
	_ = js2

	//fmt.Println(string(js1))
	//fmt.Println(string(js2))

	require.Equal(t, eval, dc1)
}

func TestTypeMappingDecodeMap(t *testing.T) {
	eval := &evaluatorMap{
		Exprs: map[string]expr{
			"never": nand([]expr{
				nboolean(true),
				nboolean(false),
			}),
			"always": nand([]expr{
				nboolean(true),
				nboolean(true),
			}),
			"true": nboolean(true),
		},
	}
	enc := (&Config{
		SortMapKeys: true,
	}).Froze()
	js1, err := enc.Marshal(eval)
	require.NoError(t, err)
	//fmt.Println(string(js1))

	cfg := &Config{
		SortMapKeys: false,
		TagKey:      "json",
	}
	api := cfg.Froze()
	// add extension
	typeMap := &TypeMapping{
		Interface: reflect.TypeOf((*expr)(nil)),
		TypeField: "type",
		SubTypes: map[string]string{
			"and":  "jsoniter.and",
			"or":   "jsoniter.or",
			"bool": "jsoniter.boolean",
		},
	}
	ext, err := NewTypeMappingExt(typeMap)
	require.NoError(t, err)
	api.RegisterExtension(ext)

	dc1 := &evaluatorMap{}
	err = api.Unmarshal(js1, dc1)
	require.NoError(t, err)

	js2, err := enc.Marshal(dc1)
	require.NoError(t, err)
	_ = js2

	//fmt.Println(string(js1))
	//fmt.Println(string(js2))

	require.Equal(t, eval, dc1)
}

func TestMapKeysAsString(t *testing.T) {

	// for serialisation using value or pointer does not matter
	mp := map[SpecKeyIf]string{
		newSpecKeyVideoVal(1, 2):      "video1",
		newSpecKeyVideoVal(1, 3):      "video2",
		newSpecKeyAudioVal(1):         "audio1",
		newSpecKeyAudioVal(2):         "audio2",
		newSpecKeySubtitleVal("hey"):  "title1",
		newSpecKeySubtitleVal("hell"): "title2",
	}

	expectedMap := map[string]string{}
	encNoExt := (&Config{}).Froze()
	for k, v := range mp {
		bb, err := encNoExt.Marshal(k)
		require.NoError(t, err)
		expectedMap[string(bb)] = v
	}

	iType := reflect.TypeOf((*SpecKeyIf)(nil))            //*jsoniter.SpecKeyIf
	intfType := reflect2.ConfigUnsafe.Type2(iType.Elem()) // jsoniter.SpecKeyIf

	concreteTypes := []reflect2.Type{
		reflect2.TypeByName("jsoniter.SpecKeyVideo"),
		reflect2.TypeByName("jsoniter.SpecKeyAudio"),
		reflect2.TypeByName("jsoniter.SpecKeySubtitle"),
	}

	encIntf := (&Config{}).Froze()
	encIntf.RegisterExtension(NewKeyAsStringEncoderExt(encIntf, intfType))

	encStructs := (&Config{}).Froze()
	encStructs.RegisterExtension(NewKeyAsStringEncoderExt(encIntf, nil, concreteTypes...))

	type testCase struct {
		name string
		enc  API
	}
	for _, tcase := range []*testCase{
		{name: "with interface", enc: encIntf},
		{name: "with structs", enc: encStructs},
	} {
		js1, err := tcase.enc.Marshal(mp)
		require.NoError(t, err, tcase.name)

		mp2 := map[string]string{}
		err = encNoExt.Unmarshal(js1, &mp2)
		require.NoError(t, err, tcase.name)
		require.Equal(t, expectedMap, mp2, tcase.name)
	}

	//fmt.Println(string(js1))
	// {"{\"media_type\":\"audio\",\"channels\":1}":"audio1",
	//  "{\"media_type\":\"audio\",\"channels\":2}":"audio2",
	//  "{\"media_type\":\"subtitle\",\"Value\":\"hell\"}":"title2",
	//  "{\"media_type\":\"subtitle\",\"Value\":\"hey\"}":"title1",
	//  "{\"media_type\":\"video\",\"height\":1,\"width\":2}":"video1",
	//  "{\"media_type\":\"video\",\"height\":1,\"width\":3}":"video2"}
}

func TestTypeMappingDecodeMapKeys(t *testing.T) {
	// note use of values rather than pointer for key: a map needs to use
	// comparable as keys. If pointer to struct is used, the map won't work..
	mp := map[SpecKeyIf]string{
		newSpecKeyVideoVal(1, 2):      "video1",
		newSpecKeyVideoVal(1, 3):      "video2",
		newSpecKeyAudioVal(1):         "audio1",
		newSpecKeyAudioVal(2):         "audio2",
		newSpecKeySubtitleVal("hey"):  "title1",
		newSpecKeySubtitleVal("hell"): "title2",
	}

	// encode key as strings
	iType := reflect.TypeOf((*SpecKeyIf)(nil))            //*jsoniter.SpecKeyIf
	intfType := reflect2.ConfigUnsafe.Type2(iType.Elem()) // jsoniter.SpecKeyIf
	enc := (&Config{}).Froze()
	enc.RegisterExtension(NewKeyAsStringEncoderExt(enc, intfType))

	js1, err := enc.Marshal(mp)
	require.NoError(t, err)

	// decode
	cfg := &Config{
		SortMapKeys: false,
		TagKey:      "json",
	}
	api := cfg.Froze()
	// add extension
	typeMap := &TypeMapping{
		Interface: reflect.TypeOf((*SpecKeyIf)(nil)),
		TypeField: "media_type",
		SubTypes: map[string]string{
			"video":    "jsoniter.SpecKeyVideo",
			"audio":    "jsoniter.SpecKeyAudio",
			"subtitle": "jsoniter.SpecKeySubtitle",
		},
	}
	ext, err := NewTypeMappingExt(typeMap)
	require.NoError(t, err)
	api.RegisterExtension(ext)

	mp1 := map[SpecKeyIf]string{}
	err = api.Unmarshal(js1, &mp1)
	require.NoError(t, err)
	require.Equal(t, len(mp1), len(mp))
	require.Equal(t, mp, mp1)
}

//
// ----- test types -----
//

type Intf interface {
	Do()
}

type Upper struct {
	Cplx *Cplx `json:"cplx"`
}

type Cplx struct {
	Intf Intf `json:"intf"`
}

type Multi struct {
	Right Intf `json:"right"`
	Left  Intf `json:"left"`
}

type ImplA struct {
	Type string `json:"type"`
	A    string `json:"a"`
}

func NewA(a string) *ImplA {
	return &ImplA{Type: "ImplA", A: a}
}

func (s *ImplA) Do() {}

type ImplB struct {
	Type string `json:"type"`
	B    string `json:"b"`
}

func NewB(b string) *ImplB {
	return &ImplB{Type: "ImplB", B: b}
}

func (s *ImplB) Do() {}

type NestedOwner struct {
	Nested Intf `json:"nested"`
}

type Nested struct {
	Type string `json:"type"`
	B    string `json:"b"`
	C    *Cplx  `json:"cplx"`
	D    Intf   `json:"d"`
}

func newNested(b string) *Nested {
	return &Nested{Type: "Nested", B: b}
}

func (s *Nested) Do() {}

// ----- recursive test types -----
type Recursor struct {
	Expr expr `json:"expr"`
}

type Sub1 struct {
	Type string `json:"type"`
	Expr expr   `json:"expr"`
}

func nsub1(e expr) *Sub1 {
	return &Sub1{
		Type: "sub1",
		Expr: e,
	}
}

func (s *Sub1) Eval() interface{} { return nil }

type Sub2 struct {
	Type string `json:"type"`
	Expr expr   `json:"expr"`
}

func nsub2(e expr) *Sub2 {
	return &Sub2{
		Type: "sub2",
		Expr: e,
	}
}

func (s *Sub2) Eval() interface{} { return nil }

type evaluator struct {
	Expr expr `json:"expr"`
}

type evaluatorMap struct {
	Exprs map[string]expr `json:"exprs"`
}

type expr interface {
	Eval() interface{}
}

func nand(exprs []expr) *and {
	return &and{
		Type:  "and",
		Exprs: exprs,
	}
}

type and struct {
	Type  string `json:"type"`
	Exprs []expr `json:"exprs"`
}

func (a *and) Eval() interface{} { return nil }

func nor(exprs []expr) *or {
	return &or{
		Type:  "or",
		Exprs: exprs,
	}
}

type or struct {
	Type  string `json:"type"`
	Exprs []expr `json:"exprs"`
}

func (a *or) Eval() interface{} { return nil }

func nboolean(v bool) *boolean {
	return &boolean{
		Type: "bool",
		Val:  v,
	}
}

type boolean struct {
	Type string `json:"type"`
	Val  bool   `json:"val"`
}

func (a *boolean) Eval() interface{} { return nil }

// --- map key tests

// interface for keys
type SpecKeyIf interface {
	SpecType() SpecType
}

type SpecType string

const (
	Audio    SpecType = "audio"
	Subtitle SpecType = "subtitle"
	Video    SpecType = "video"
)

type SpecKeyAudio struct {
	Type     SpecType `json:"media_type"`
	Channels int      `json:"channels"`
}

// use value rather than pointer for use as map key
func newSpecKeyAudioVal(c int) SpecKeyAudio {
	return SpecKeyAudio{Type: Audio, Channels: c}
}

func (lsk SpecKeyAudio) SpecType() SpecType {
	return lsk.Type
}

type SpecKeySubtitle struct {
	Type  SpecType `json:"media_type"`
	Value string
}

// use value rather than pointer for use as map key
func newSpecKeySubtitleVal(s string) SpecKeySubtitle {
	return SpecKeySubtitle{Type: Subtitle, Value: s}
}

func (lsk SpecKeySubtitle) SpecType() SpecType {
	return lsk.Type
}

type SpecKeyVideo struct {
	Type   SpecType `json:"media_type"`
	Height int      `json:"height"`
	Width  int      `json:"width"`
}

// use value rather than pointer for use as map key
func newSpecKeyVideoVal(h, w int) SpecKeyVideo {
	return SpecKeyVideo{Type: Video, Height: h, Width: w}
}

func (lsk SpecKeyVideo) SpecType() SpecType {
	return lsk.Type
}
