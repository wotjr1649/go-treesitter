package main

import (
	"reflect"
	"testing"
)

func TestSelectedLanguages(t *testing.T) {
	got, err := selectedLanguages("cpp, javascript,cpp,rust")
	if err != nil || !reflect.DeepEqual(got, []string{"cpp", "javascript", "rust"}) {
		t.Fatalf("selectedLanguages = %v, %v", got, err)
	}
	if _, err := selectedLanguages("cpp,typo"); err == nil {
		t.Fatal("unknown language accepted")
	}
	all, err := selectedLanguages("")
	if err != nil || len(all) != 15 {
		t.Fatalf("all languages = %v, %v", all, err)
	}
}
