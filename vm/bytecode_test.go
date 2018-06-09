// Copyright 2018 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package vm

import (
	"testing"

	go_cmp "github.com/google/go-cmp/cmp"
)

func TestBytecodeString(t *testing.T) {
	expected := "{match 0}"

	if diff := go_cmp.Diff(instr{match, 0}.String(), expected); diff != "" {
		t.Errorf("bytedoce string didn't match:\n%s", diff)
	}
}
