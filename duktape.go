package duktape

/*
#cgo linux LDFLAGS: -lm

# include "duktape.h"
extern duk_ret_t goFinalize(duk_context *ctx);
extern duk_ret_t goCall(duk_context *ctx);
extern void goFatalError(void* udata, int code, const char* msg);
*/
import "C"
import (
	"errors"
	"log"
	"sync"
	"unsafe"
)

const goFuncProp = "goFuncData"
const goObjProp = "goObjData"
const (
	DUK_ENUM_INCLUDE_NONENUMERABLE = C.DUK_ENUM_INCLUDE_NONENUMERABLE
	DUK_ENUM_INCLUDE_INTERNAL      = C.DUK_ENUM_INCLUDE_INTERNAL
	DUK_ENUM_OWN_PROPERTIES_ONLY   = C.DUK_ENUM_OWN_PROPERTIES_ONLY
	DUK_ENUM_ARRAY_INDICES_ONLY    = C.DUK_ENUM_ARRAY_INDICES_ONLY
	DUK_ENUM_SORT_ARRAY_INDICES    = C.DUK_ENUM_SORT_ARRAY_INDICES
	DUK_ENUM_NO_PROXY_BEHAVIOR     = C.DUK_ENUM_NO_PROXY_BEHAVIOR
)

const (
	DUK_RET_UNIMPLEMENTED_ERROR = C.DUK_RET_UNIMPLEMENTED_ERROR
	DUK_RET_UNSUPPORTED_ERROR   = C.DUK_RET_UNSUPPORTED_ERROR
	DUK_RET_INTERNAL_ERROR      = C.DUK_RET_INTERNAL_ERROR
	DUK_RET_ALLOC_ERROR         = C.DUK_RET_ALLOC_ERROR
	DUK_RET_ASSERTION_ERROR     = C.DUK_RET_ASSERTION_ERROR
	DUK_RET_API_ERROR           = C.DUK_RET_API_ERROR
	DUK_RET_UNCAUGHT_ERROR      = C.DUK_RET_UNCAUGHT_ERROR
	DUK_RET_ERROR               = C.DUK_RET_ERROR
	DUK_RET_EVAL_ERROR          = C.DUK_RET_EVAL_ERROR
	DUK_RET_RANGE_ERROR         = C.DUK_RET_RANGE_ERROR
	DUK_RET_REFERENCE_ERROR     = C.DUK_RET_REFERENCE_ERROR
	DUK_RET_SYNTAX_ERROR        = C.DUK_RET_SYNTAX_ERROR
	DUK_RET_TYPE_ERROR          = C.DUK_RET_TYPE_ERROR
	DUK_RET_URI_ERROR           = C.DUK_RET_URI_ERROR
	DUK_RET_INSTACK_ERROR       = C.DUK_RET_INSTACK_ERROR
)

const (
	DUK_TYPE_NONE Type = iota
	DUK_TYPE_UNDEFINED
	DUK_TYPE_NULL
	DUK_TYPE_BOOLEAN
	DUK_TYPE_NUMBER
	DUK_TYPE_STRING
	DUK_TYPE_OBJECT
	DUK_TYPE_BUFFER
	DUK_TYPE_POINTER
)

const (
	DUK_COMPILE_EVAL     = C.DUK_COMPILE_EVAL
	DUK_COMPILE_FUNCTION = C.DUK_COMPILE_FUNCTION
	DUK_COMPILE_STRICT   = C.DUK_COMPILE_STRICT
)

const (
	DUK_ERR_UNIMPLEMENTED_ERROR = C.DUK_ERR_UNIMPLEMENTED_ERROR
	DUK_ERR_UNSUPPORTED_ERROR   = C.DUK_ERR_UNSUPPORTED_ERROR
	DUK_ERR_INTERNAL_ERROR      = C.DUK_ERR_INTERNAL_ERROR
	DUK_ERR_ALLOC_ERROR         = C.DUK_ERR_ALLOC_ERROR
	DUK_ERR_ASSERTION_ERROR     = C.DUK_ERR_ASSERTION_ERROR
	DUK_ERR_API_ERROR           = C.DUK_ERR_API_ERROR
	DUK_ERR_UNCAUGHT_ERROR      = C.DUK_ERR_UNCAUGHT_ERROR
	DUK_ERR_ERROR               = C.DUK_ERR_ERROR
	DUK_ERR_EVAL_ERROR          = C.DUK_ERR_EVAL_ERROR
	DUK_ERR_RANGE_ERROR         = C.DUK_ERR_RANGE_ERROR
	DUK_ERR_REFERENCE_ERROR     = C.DUK_ERR_REFERENCE_ERROR
	DUK_ERR_SYNTAX_ERROR        = C.DUK_ERR_SYNTAX_ERROR
	DUK_ERR_TYPE_ERROR          = C.DUK_ERR_TYPE_ERROR
	DUK_ERR_URI_ERROR           = C.DUK_ERR_URI_ERROR
	DUK_ERR_INSTACK_ERROR       = C.DUK_ERR_INSTACK_ERROR
)

type Type int

func (t Type) IsNone() bool      { return t == DUK_TYPE_NONE }
func (t Type) IsUndefined() bool { return t == DUK_TYPE_UNDEFINED }
func (t Type) IsNull() bool      { return t == DUK_TYPE_NULL }
func (t Type) IsBool() bool      { return t == DUK_TYPE_BOOLEAN }
func (t Type) IsNumber() bool    { return t == DUK_TYPE_NUMBER }
func (t Type) IsString() bool    { return t == DUK_TYPE_STRING }
func (t Type) IsObject() bool    { return t == DUK_TYPE_OBJECT }
func (t Type) IsBuffer() bool    { return t == DUK_TYPE_BUFFER }
func (t Type) IsPointer() bool   { return t == DUK_TYPE_POINTER }

var objectMutex sync.Mutex
var objectMap map[unsafe.Pointer]interface{} = make(map[unsafe.Pointer]interface{})

type Context struct {
	duk_context unsafe.Pointer
}

// Returns initialized duktape context object
func NewContext() *Context {
	ctx := &Context{
		// TODO: "A caller SHOULD implement a fatal error handler in most applications."
		duk_context: C.duk_create_heap(nil, nil, nil, nil, (*[0]byte)(C.goFatalError)),
	}
	return ctx
}

func (d *Context) PutInternalPropString(objIndex int, key string) bool {
	cKey := C.CString("\xff" + key) // \xff as the first char designates an internal property
	defer C.free(unsafe.Pointer(cKey))
	return int(C.duk_put_prop_string(d.duk_context, C.duk_idx_t(objIndex), cKey)) == 1
}

func (d *Context) GetInternalPropString(objIndex int, key string) bool {
	cKey := C.CString("\xff" + key) // \xff as the first char designates an internal property
	defer C.free(unsafe.Pointer(cKey))
	return int(C.duk_get_prop_string(d.duk_context, C.duk_idx_t(objIndex), cKey)) == 1
}

//export goFinalize
func goFinalize(ctx unsafe.Pointer) C.duk_ret_t {
	d := &Context{ctx}
	d.PushCurrentFunction()
	d.GetInternalPropString(-1, goFuncProp)
	if !Type(d.GetType(-1)).IsPointer() {
		d.Pop2()
		return C.duk_ret_t(C.DUK_RET_TYPE_ERROR)
	}
	key := d.GetPointer(-1)
	d.Pop2()
	objectMutex.Lock()
	delete(objectMap, key)
	objectMutex.Unlock()
	C.free(key)
	return C.duk_ret_t(0)
}

func (d *Context) putGoObjectRef(prop string, o interface{}) {
	key := C.malloc(1) // guaranteed to be unique until freed

	objectMutex.Lock()
	objectMap[key] = o
	objectMutex.Unlock()

	d.PushCFunction((*[0]byte)(C.goFinalize), 1)
	d.PushPointer(key)
	d.PutInternalPropString(-2, goFuncProp)

	d.SetFinalizer(-2)

	d.PushPointer(key)
	d.PutInternalPropString(-2, prop)
}

func (d *Context) PushGoObject(o interface{}) {
	d.PushObject()
	d.putGoObjectRef(goObjProp, o)
}

func (d *Context) getGoObjectRef(objIndex int, prop string) interface{} {
	d.GetInternalPropString(objIndex, prop)
	if !Type(d.GetType(-1)).IsPointer() {
		d.Pop()
		return nil
	}
	key := d.GetPointer(-1)
	d.Pop()
	objectMutex.Lock()
	defer objectMutex.Unlock()
	return objectMap[key]
}

func (d *Context) GetGoObject(objIndex int) interface{} {
	return d.getGoObjectRef(objIndex, goObjProp)
}

//export goCall
func goCall(ctx unsafe.Pointer) C.duk_ret_t {
	d := &Context{ctx}

	/*
		d.PushContextDump()
		log.Printf("goCall context: %s", d.GetString(-1))
		d.Pop()
	*/

	d.PushCurrentFunction()
	if fd, _ := d.getGoObjectRef(-1, goFuncProp).(*GoFuncData); fd == nil {
		d.Pop()
		return C.duk_ret_t(C.DUK_RET_TYPE_ERROR)
	} else {
		d.Pop()
		return C.duk_ret_t(fd.f(d))
	}
}

type GoFunc func(d *Context) int
type GoFuncData struct {
	f GoFunc
}

// Push goCall with its "goFuncData" property set to fd
func (d *Context) PushGoFunc(f GoFunc) {
	fd := &GoFuncData{f}
	d.PushCFunction((*[0]byte)(C.goCall), C.DUK_VARARGS)
	d.putGoObjectRef(goFuncProp, fd)
}

type MethodSuite map[string]GoFunc

func (d *Context) EvalWith(source string, suite MethodSuite) error {
	if err := d.PevalString(source); err != 0 {
		return errors.New(d.SafeToString(-1))
	}

	d.PushObject()

	for prop, f := range suite {
		d.PushGoFunc(f)
		d.PutPropString(-2, prop)
	}

	if err := d.Pcall(1); err != 0 {
		return errors.New(d.SafeToString(-1))
	}

	return nil
}

//export goFatalError
func goFatalError(udata unsafe.Pointer, code C.int, msg *C.char) {
	log.Panicf("duktape fatal: [%d] %s", code, C.GoString(msg))
}

// TBD: panic handling.
// When a goroutine panics, mark the context as panicking and throw a special JS
// exception, saving the value passed to panic() to the context.
// When the exception is caught, retrieve the value that was passed to panic(),
// and call panic() again.
