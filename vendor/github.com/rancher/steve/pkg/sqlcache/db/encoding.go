package db

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/tinylib/msgp/msgp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Encoding int

const (
	GobEncoding Encoding = iota
	JSONEncoding
	MsgpackEncoding
	GzippedGobEncoding
	GzippedJSONEncoding
)

var defaultEncoding = encodingForType(MsgpackEncoding)

type nonNilEmptySlice struct{}

func init() {
	// necessary in order to gob/ungob unstructured.Unstructured objects
	gob.Register(map[string]any{})
	gob.Register([]any{})
	gob.Register(nonNilEmptySlice{})

	// Allow using JSON encoding during development
	switch os.Getenv("CATTLE_SQL_CACHE_ENCODING") {
	case "gob":
		defaultEncoding = encodingForType(GobEncoding)
	case "json":
		defaultEncoding = encodingForType(JSONEncoding)
	case "gob+gz":
		defaultEncoding = encodingForType(GzippedGobEncoding)
	case "json+gz":
		defaultEncoding = encodingForType(GzippedJSONEncoding)
	}
}

func encodingForType(encType Encoding) encoding {
	switch encType {
	case GobEncoding:
		return &gobEncoding{}
	case JSONEncoding:
		return jsonEncoding{}
	case MsgpackEncoding:
		return msgpackEncoding{}
	case GzippedGobEncoding:
		return gzipped(&gobEncoding{})
	case GzippedJSONEncoding:
		return gzipped(jsonEncoding{})
	}
	// unreachable
	return msgpackEncoding{}
}

func WithEncoding(encType Encoding) ClientOption {
	return func(c *client) {
		c.encoding = encodingForType(encType)
	}
}

type encoding interface {
	// Encode serializes an object into the provided writer
	Encode(io.Writer, any) error
	// Decode reads from a provided reader and deserializes into an object
	Decode(io.Reader, any) error
}

type gobEncoding struct {
	writeLock, readLock sync.Mutex
	writeBuf, readBuf   bytes.Buffer
	encoder             *gob.Encoder
	decoder             *gob.Decoder
	seenTypes           map[reflect.Type]struct{}
}

func (g *gobEncoding) Encode(w io.Writer, obj any) error {
	g.writeLock.Lock()
	defer g.writeLock.Unlock()

	if g.encoder == nil {
		g.encoder = gob.NewEncoder(&g.writeBuf)
	}
	g.writeBuf.Reset()

	// Apply modifications to bypass limitations in the gob encoding, which will need to be restored when decoding
	if u, ok := obj.(*unstructured.Unstructured); ok {
		obj = &unstructured.Unstructured{Object: applyPatches(u.Object)}
	}

	// gob encoders and decoders share extra types information the first time a certain object type is transferred
	if err := g.registerTypeIfNeeded(obj); err != nil {
		return err
	}

	// Encode to the internal write buffer
	if err := g.encoder.Encode(obj); err != nil {
		return err
	}

	// Finally copy from internal buffer to the destination
	_, err := g.writeBuf.WriteTo(w)
	return err
}

// registerTypeIfNeeded prevents future decoding errors by running Decode right after the first Encode for an object type
// This is needed when reusing a gob.Encoder, as it assumes the receiving end of those messages is always the same gob.Decoder.
// Due to this assumption, it applies some optimizations, like avoiding sending complete type's information (needed for decoding) if it already did it before.
// This means the first object for each type encoded by a gob.Encoder will have a bigger payload, but more importantly,
// the decoding will fail if this is not the first object being decoded later. This function forces that to prevent this from happening.
func (g *gobEncoding) registerTypeIfNeeded(obj any) error {
	if g.seenTypes == nil {
		g.seenTypes = make(map[reflect.Type]struct{})

		// gob seems to get confused with arbitrary lists, especially if they contain nested objects. Make sure the proper compiler is created
		if err := g.registerTypeIfNeeded([]any{map[string]any{}}); err != nil {
			return nil
		}
	}

	typ := reflect.TypeOf(obj)
	if _, ok := g.seenTypes[typ]; ok {
		return nil
	}

	if err := g.encoder.Encode(obj); err != nil {
		return err
	}
	defer g.writeBuf.Reset()

	// Decode into a new object to avoid modifying the original. This let the decoder consume the extra headers sent by the encoder only the first time
	newObj := reflect.New(typ).Interface()
	if err := g.Decode(bytes.NewReader(g.writeBuf.Bytes()), newObj); err != nil {
		return fmt.Errorf("could not decode %T: %w", obj, err)
	}

	g.seenTypes[typ] = struct{}{}
	return nil
}

func (g *gobEncoding) Decode(r io.Reader, into any) error {
	g.readLock.Lock()
	defer g.readLock.Unlock()

	if g.decoder == nil {
		g.decoder = gob.NewDecoder(&g.readBuf)
	}

	g.readBuf.Reset()
	if _, err := g.readBuf.ReadFrom(r); err != nil {
		return err
	}

	if err := g.decoder.Decode(into); err != nil {
		return err
	}

	// Undo the changes applied by applyPatches
	if u, ok := into.(*unstructured.Unstructured); ok {
		u.Object = revertPatches(u.Object)
	}

	return nil
}

type jsonEncoding struct {
	indentLevel int
}

func (j jsonEncoding) Encode(w io.Writer, obj any) error {
	enc := json.NewEncoder(w)
	if j.indentLevel > 0 {
		enc.SetIndent("", strings.Repeat(" ", j.indentLevel))
	}

	return enc.Encode(obj)
}

func (j jsonEncoding) Decode(r io.Reader, into any) error {
	return json.NewDecoder(r).Decode(into)
}

type msgpackEncoding struct{}

func (m msgpackEncoding) Encode(w io.Writer, obj any) error {
	if e, ok := obj.(msgp.Encodable); ok {
		return msgp.Encode(w, e)
	}

	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("only *unstructured.Unstructured is currently supported, got %T", obj)
	}
	return msgp.Encode(w, unstructuredObject(u.Object))
}

func (m msgpackEncoding) Decode(r io.Reader, into any) error {
	if d, ok := into.(msgp.Decodable); ok {
		return msgp.Decode(r, d)
	}

	u, ok := into.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("only *unstructured.Unstructured is currently supported, got %T", into)
	}
	var obj unstructuredObject
	if err := msgp.Decode(r, &obj); err != nil {
		return err
	}
	u.Object = obj
	return nil
}

type gzipEncoding struct {
	encoding
	writers sync.Pool
	readers sync.Pool
}

func gzipped(wrapped encoding) *gzipEncoding {
	gz := gzipEncoding{
		encoding: wrapped,
	}
	return &gz
}

func (gz *gzipEncoding) Encode(w io.Writer, obj any) error {
	gzw, ok := gz.writers.Get().(*gzip.Writer)
	if !ok {
		gzw = gzip.NewWriter(io.Discard)
	}
	gzw.Reset(w)
	defer func() {
		gzw.Reset(nil)
		gz.writers.Put(gzw)
	}()

	if err := gz.encoding.Encode(gzw, obj); err != nil {
		return err
	}
	return gzw.Close()
}

func (gz *gzipEncoding) Decode(r io.Reader, into any) error {
	gzr, ok := gz.readers.Get().(*gzip.Reader)
	if ok {
		if err := gzr.Reset(r); err != nil {
			return err
		}
	} else {
		var err error
		gzr, err = gzip.NewReader(r)
		if err != nil {
			return err
		}
	}
	defer func() {
		gzr.Close()
		gz.readers.Put(gzr)
	}()

	if err := gz.encoding.Decode(gzr, into); err != nil {
		return err
	}
	return gzr.Close()
}

// isSameMap compares if two maps are the same (not their contents)
func isSameMap[K comparable, V any](m1, m2 map[K]V) bool {
	return reflect.ValueOf(m1).Pointer() == reflect.ValueOf(m2).Pointer()
}

func isSameSlice(s1, s2 []any) bool {
	return reflect.ValueOf(s1).Pointer() == reflect.ValueOf(s2).Pointer()
}

// applyPatches modifies the data from an unstructured object before storing in the database to bypass known limitations in gob encoding
func applyPatches(data map[string]any) map[string]any {
	// Minimize allocations by only cloning data if really needed
	var updated map[string]any
	update := func(k string, v any) {
		if updated == nil {
			updated = maps.Clone(data)
		}
		updated[k] = v
	}
	for k := range data {
		switch v := data[k].(type) {
		case map[string]any:
			if len(v) != 0 {
				if newValue := applyPatches(v); !isSameMap(v, newValue) {
					update(k, newValue)
				}
			}
		case []any:
			// gob cannot differentiate between nil or non-nil empty slices, so it converts empty slices into null
			// https://github.com/golang/go/issues/10905
			if v != nil && len(v) == 0 {
				update(k, nonNilEmptySlice{})
			} else if len(v) > 0 {
				if newValue := applyPatchesToSlice(v); !isSameSlice(v, newValue) {
					update(k, newValue)
				}
			}
		}
	}
	if updated == nil {
		return data
	}
	return updated
}

func applyPatchesToSlice(data []any) []any {
	// Minimize allocations by only cloning data if really needed
	var updated []any
	update := func(x int, v any) {
		if updated == nil {
			updated = slices.Clone(data)
		}
		updated[x] = v
	}

	for x := range data {
		switch v := data[x].(type) {
		case map[string]any:
			// Rule 1: Recurse into non-empty maps
			if len(v) > 0 {
				if newValue := applyPatches(v); !isSameMap(v, newValue) {
					update(x, newValue)
				}
			}
		case []any:
			if v != nil && len(v) == 0 {
				update(x, nonNilEmptySlice{})
			} else if len(v) > 0 {
				if newValue := applyPatchesToSlice(v); !isSameSlice(v, newValue) {
					update(x, newValue)
				}
			}
		}
	}

	if updated == nil {
		// no changes
		return data
	}
	return updated
}

// revertPatches is meant to restore the changes performed by applyPatches
func revertPatches(data map[string]any) map[string]any {
	for k := range data {
		switch v := data[k].(type) {
		case map[string]any:
			data[k] = revertPatches(v)
		case []any:
			data[k] = revertPatchesInSlice(v)
		case nonNilEmptySlice:
			data[k] = []any{}
		}
	}
	return data
}

func revertPatchesInSlice(data []any) []any {
	for k := range data {
		switch v := data[k].(type) {
		case map[string]any:
			data[k] = revertPatches(v)
		case []any:
			data[k] = revertPatchesInSlice(v)
		case nonNilEmptySlice:
			data[k] = []any{}
		}
	}
	return data
}
