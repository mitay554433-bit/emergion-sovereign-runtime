package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/pkg/fieldapi"
)

func cString(s string) *C.char {
	return C.CString(s)
}

//export FieldStatusJSON
func FieldStatusJSON(stateRoot *C.char) *C.char {
	if stateRoot == nil {
		return cString(`{"error":"state root required"}`)
	}

	rt, err := fieldapi.Open(
		C.GoString(stateRoot),
		reason.GemmaFromEnv(),
	)
	if err != nil {
		return cString(`{"error":"runtime open failed"}`)
	}

	wire, err := rt.StatusJSON()
	if err != nil {
		return cString(`{"error":"status failed"}`)
	}

	return cString(wire)
}

//export FieldFree
func FieldFree(p unsafe.Pointer) {
	C.free(p)
}

func main() {}

//export FieldActionsJSON
func FieldActionsJSON(
	stateRoot *C.char,
	emergionID *C.char,
	localGemma C.int,
) *C.char {
	if stateRoot == nil || emergionID == nil {
		return cString(`{"error":"state root and emergion id required"}`)
	}

	rt, err := fieldapi.Open(
		C.GoString(stateRoot),
		reason.GemmaFromEnv(),
	)
	if err != nil {
		return cString(`{"error":"runtime open failed"}`)
	}

	wire, err := rt.ActionsJSON(
		C.GoString(emergionID),
		localGemma != 0,
	)
	if err != nil {
		return cString(`{"error":"actions unavailable"}`)
	}

	return cString(wire)
}

//export FieldDecideBinding
func FieldDecideBinding(
	stateRoot *C.char,
	id *C.char,
	decision *C.char,
	reasonText *C.char,
) *C.char {
	if stateRoot == nil || id == nil || decision == nil {
		return cString(`{"error":"state root, id, and decision required"}`)
	}

	rt, err := fieldapi.Open(
		C.GoString(stateRoot),
		reason.GemmaFromEnv(),
	)
	if err != nil {
		return cString(`{"error":"runtime open failed"}`)
	}

	rText := ""
	if reasonText != nil {
		rText = C.GoString(reasonText)
	}

	err = rt.DecideBinding(
		C.GoString(id),
		C.GoString(decision),
		rText,
	)
	if err != nil {
		return cString(`{"error":"decision failed"}`)
	}

	return cString(`{"status":"success"}`)
}

//export FieldAuthorizeBinding
func FieldAuthorizeBinding(
	stateRoot *C.char,
	emergionID *C.char,
	adapter *C.char,
	action *C.char,
	reasonText *C.char,
	localGemma C.int,
) *C.char {
	if stateRoot == nil || emergionID == nil || adapter == nil || action == nil {
		return cString(`{"error":"state root, id, adapter, and action required"}`)
	}

	rt, err := fieldapi.Open(
		C.GoString(stateRoot),
		reason.GemmaFromEnv(),
	)
	if err != nil {
		return cString(`{"error":"runtime open failed"}`)
	}

	rText := ""
	if reasonText != nil {
		rText = C.GoString(reasonText)
	}

	authJSON, err := rt.AuthorizeBinding(
		C.GoString(emergionID),
		C.GoString(adapter),
		C.GoString(action),
		rText,
		localGemma != 0,
	)
	if err != nil {
		return cString(`{"error":"authorization failed"}`)
	}

	return cString(authJSON)
}
