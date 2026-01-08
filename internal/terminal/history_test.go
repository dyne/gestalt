package terminal

import (
	"reflect"
	"testing"
)

func TestMergeHistoryLines(t *testing.T) {
	fileLines := []string{"one", "two", "three"}
	bufferLines := []string{"three", "four"}

	got := mergeHistoryLines(fileLines, bufferLines, 10)
	expected := []string{"one", "two", "three", "four"}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}
