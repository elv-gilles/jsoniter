package jsoniter

import (
	"bytes"
	"fmt"
	"github.com/modern-go/reflect2"
	"reflect"
	"strings"
	"unsafe"
)

// TypeMapping defines the configuration for dynamically decoding json objects
// into struct field or slice items defined by an interface type - similar to
// Jackson Polymorphic Types deserialization in java.
// The mapping relies on the value of some 'type field' defined in all sub types.
//
// Example
//   type Intf interface{}
//   // A implements Intf
//   type A struct{
//       Type string `json:"type"`
//   }
//   func newA() *A {
//       return &A{Type:"A"}
//   }
//   // B implements Intf
//   type B struct{
//       Type string `json:"type"`
//   }
//   func newB() *B {
//       return &B{Type:"B"}
//   }
//   type MyStruct struct {
//      Intf Intf `json:"intf"`
//   }
//
//   To de-serialize a json of MyStruct
//	 cfg := &Config{
//	 	SortMapKeys: false,
//	 	TagKey:      "json",
//	 }
//	 api := cfg.Froze()
//	 ext, err := NewTypeMappingExt(&TypeMapping{
//		 Interface: reflect.TypeOf((*Intf)(nil)),
//		 TypeField: "type",
//		 SubTypes: map[string]string{
//			 "A": "my_package.A",
//			 "B": "my_package.B",
//		 },
//	 })
//   if err != nil { .. }
//	 api.RegisterExtension(ext)
//   Then use 'api' as usual.
//
type TypeMapping struct {
	Interface reflect.Type      `json:"interface"`  // target interface
	TypeField string            `json:"type_field"` // name of the json field holding type information
	SubTypes  map[string]string `json:"sub_types"`  // map value of the type_field to qualified names of sub types
}

func NewTypeMappingExt(cfgs ...*TypeMapping) (*TypeMappingExt, error) {
	if len(cfgs) == 0 {
		return nil, fmt.Errorf("no config provided")
	}
	types := map[reflect2.Type]ValDecoder{}
	for _, cfg := range cfgs {
		if cfg.Interface == nil {
			return nil, fmt.Errorf("nil interface provided")
		}
		var intfType reflect2.Type
		switch cfg.Interface.Kind() {
		case reflect.Ptr:
			intfType = reflect2.ConfigUnsafe.Type2(cfg.Interface.Elem())
		case reflect.Interface:
			intfType = reflect2.ConfigUnsafe.Type2(cfg.Interface)
		default:
			return nil, fmt.Errorf("invalid interface %s (kind %s)",
				cfg.Interface.String(),
				cfg.Interface.Kind())
		}

		var err error
		types[intfType], err = newTypeMapping(intfType, cfg)
		if err != nil {
			return nil, err
		}
	}
	return &TypeMappingExt{types: types}, nil
}

var _ Extension = (*TypeMappingExt)(nil)

type TypeMappingExt struct {
	types map[reflect2.Type]ValDecoder
}

func (e *TypeMappingExt) UpdateStructDescriptor(_ *StructDescriptor) {
}

func (e *TypeMappingExt) CreateMapKeyDecoder(typ reflect2.Type) ValDecoder {
	return e.types[typ]
}

func (e *TypeMappingExt) CreateDecoder(typ reflect2.Type) ValDecoder {
	return e.types[typ]
}

func (e *TypeMappingExt) DecorateDecoder(_ reflect2.Type, decoder ValDecoder) ValDecoder {
	return decoder
}

func (e *TypeMappingExt) CreateMapKeyEncoder(_ reflect2.Type) ValEncoder {
	return nil
}

func (e *TypeMappingExt) CreateEncoder(_ reflect2.Type) ValEncoder {
	return nil
}

func (e *TypeMappingExt) DecorateEncoder(_ reflect2.Type, encoder ValEncoder) ValEncoder {
	return encoder
}

// ----- typeMapping Decoder -----
type typeMapping struct {
	interfaceType *reflect2.UnsafeIFaceType // the interface
	typeField     string                    // name of the json field holding type information
	subTypes      map[string]reflect2.Type  // map value of the type_field to types
}

// mappingCtx is contextual information for type mapping decoding.
// The container information is needed for setting newly created objects.
type mappingCtx struct {
	contType   reflect2.Type        // type of container
	contPtr    unsafe.Pointer       // ptr to container
	field      reflect2.StructField // field of a struct container
	sliceIndex int                  // index in a slice container
	mapKeyType reflect2.Type        // type of key in a map container
	mapKey     unsafe.Pointer       // key in a map container
}

func (c *mappingCtx) container() interface{} {
	return c.contType.PackEFace(c.contPtr)
}

func newTypeMapping(intfType reflect2.Type, typeMap *TypeMapping) (*typeMapping, error) {
	if len(typeMap.SubTypes) == 0 {
		return nil, fmt.Errorf("no sub-types for '%s'", intfType.String())
	}
	valType, ok := intfType.(*reflect2.UnsafeIFaceType)
	if !ok {
		return nil, fmt.Errorf("invalid interface typpe '%s'", intfType.String())
	}

	subTypes := map[string]reflect2.Type{}
	for typeName, subtyp := range typeMap.SubTypes {
		subType := reflect2.TypeByName(subtyp)
		if subType == nil {
			// ignore: this happens if the sub-type is actually not in use
			//return nil, fmt.Errorf("type '%s' not found", subtyp)
			continue
		}
		if !reflect2.PtrTo(subType).Implements(intfType) {
			return nil, fmt.Errorf("sub-type '%s' does not implement %s",
				subType.String(),
				intfType.String())
		}
		subTypes[typeName] = subType
	}

	return &typeMapping{
		interfaceType: valType,
		typeField:     typeMap.TypeField,
		subTypes:      subTypes,
	}, nil
}

func (typMap *typeMapping) Decode(_ unsafe.Pointer, iter *Iterator) {
	iter.Error = fmt.Errorf("typeMapping decoder: typeMapping supports only structs, slices and maps")
}

func (typMap *typeMapping) DynDecodeKey(iter *Iterator) (interface{}, error) {

	if iter.ReadNil() {
		return nil, nil
	}

	keyString := iter.ReadString()
	if iter.Error != nil {
		return nil, fmt.Errorf("typeMapping key decoder: error [%s] reading key when decoding '%s'",
			iter.Error.Error(),
			typMap.interfaceType.String())
	}

	iterKey := iter.cfg.BorrowIterator([]byte(keyString))
	defer iter.cfg.ReturnIterator(iterKey)
	start := iterKey.head
	typeValue := locateObjectField(iterKey, typMap.typeField)
	iterKey.head = start
	if iterKey.Error != nil {
		return nil, fmt.Errorf("typeMapping key decoder: error [%s] trying to locate type field '%s' when decoding '%s'",
			iter.Error.Error(),
			typMap.typeField,
			typMap.interfaceType.String())
	}

	// create a new instance
	newObj, err := typMap.newObjectPtr(string(typeValue))
	if err != nil {
		return nil, err
	}
	iterKey.ReadVal(newObj)

	return newObj, nil
}

func (typMap *typeMapping) DynDecode(decCtx *mappingCtx, ptr unsafe.Pointer, iter *Iterator) {

	if iter.ReadNil() {
		typMap.interfaceType.UnsafeSet(ptr, typMap.interfaceType.UnsafeNew())
		return
	}
	obj := typMap.interfaceType.UnsafeIndirect(ptr)
	if reflect2.IsNil(obj) {

		// find the type field
		start := iter.head
		typeValue := locateObjectField(iter, typMap.typeField)
		iter.head = start
		if iter.Error != nil {
			iter.Error = fmt.Errorf("typeMapping decoder: error [%s] trying to locate type field '%s' when decoding '%s'",
				iter.Error.Error(),
				typMap.typeField,
				typMap.interfaceType.String())
			return
		}

		// create a new instance
		newObj, err := typMap.newObjectPtr(string(typeValue))
		if err != nil {
			iter.Error = err
			return
		}

		container := decCtx.container()
		containerElem := reflect.ValueOf(container).Elem()
		newVal := reflect.ValueOf(newObj)

		switch containerElem.Kind() {
		case reflect.Struct:
			if decCtx.field == nil {
				iter.Error = fmt.Errorf("typeMapping decoder: expecting struct field when decoding '%s'", typMap.interfaceType.String())
				return
			}
			sf1 := containerElem.FieldByName(decCtx.field.Name())
			sf1.Set(newVal)

			// was not able to make it work with reflect2
			//field.Set(container, newObj)

		case reflect.Slice:
			containerElem.Index(decCtx.sliceIndex).Set(newVal)

		case reflect.Map:
			key := decCtx.mapKeyType.PackEFace(decCtx.mapKey)
			keyElem := reflect.ValueOf(key).Elem()
			containerElem.SetMapIndex(keyElem, newVal)

		default:
			iter.Error = fmt.Errorf("typeMapping decoder: unexpected kind %s of container when decoding '%s'",
				containerElem.Kind(),
				typMap.interfaceType.String())
			return
		}

		iter.ReadVal(newObj)
		return
	}
	iter.ReadVal(obj)
}

func (typMap *typeMapping) newObjectPtr(typeName string) (interface{}, error) {
	typeName = strings.Trim(typeName, `" `)
	typ, ok := typMap.subTypes[typeName]
	if !ok || reflect2.IsNil(typ) {
		return nil, fmt.Errorf("typeMapping decoder: type '%s' not found", typeName)
	}
	newPtr := typ.UnsafeNew()
	return typ.PackEFace(newPtr), nil
}

// ----- KeyAsStringEncoderExt -----
var _ Extension = (*KeyAsStringEncoderExt)(nil)

// KeyAsStringEncoderExt is an extension to encode map keys as a json string.
// The target type may be specified either by an interface or as a list of
// concrete types to encode.
type KeyAsStringEncoderExt struct {
	DummyExtension
	ctx      API             // the configured API that will use this extension
	intfType reflect2.Type   // an interface implemented by keys
	types    []reflect2.Type // concrete types
}

func NewKeyAsStringEncoderExt(ctx API, intfType reflect2.Type, types ...reflect2.Type) *KeyAsStringEncoderExt {
	return &KeyAsStringEncoderExt{
		ctx:      ctx,
		intfType: intfType,
		types:    types,
	}
}

func (e *KeyAsStringEncoderExt) inTypes(typ reflect2.Type) bool {
	for _, t := range e.types {
		if typ == t || reflect2.PtrTo(t) == typ {
			return true
		}
	}
	return false
}

func (e *KeyAsStringEncoderExt) asInterface(typ reflect2.Type) bool {
	if e.intfType == nil {
		return false
	}
	return typ.Implements(e.intfType) || typ == e.intfType
}

func (e *KeyAsStringEncoderExt) CreateMapKeyEncoder(typ reflect2.Type) ValEncoder {
	if !e.asInterface(typ) && !e.inTypes(typ) {
		return nil
	}
	enc := e.ctx.EncoderOf(typ)
	if enc == nil {
		return nil // we should have a context to signal an error
	}

	return &keyAsStringEncoder{
		ctx: e.ctx,
		enc: enc,
	}
}

var _ ValEncoder = (*keyAsStringEncoder)(nil)

// keyAsStringEncoder uses the wrapped encoder to encode the target as json then
// output the result as a json string.
type keyAsStringEncoder struct {
	ctx API
	enc ValEncoder
}

func (e *keyAsStringEncoder) IsEmpty(ptr unsafe.Pointer) bool {
	return false
}

func (e *keyAsStringEncoder) Encode(ptr unsafe.Pointer, stream *Stream) {
	buf := bytes.NewBuffer(make([]byte, 0))
	str := NewStream(e.ctx, buf, 64*1024)
	e.enc.Encode(ptr, str)
	if str.Error != nil {
		stream.Error = str.Error
		return
	}
	err := str.Flush()
	if err != nil {
		stream.Error = err
		return
	}
	stream.WriteStringWithHTMLEscaped(string(buf.Bytes()))
}
