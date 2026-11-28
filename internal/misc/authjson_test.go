package misc

import (
	"math/bits"
	"reflect"
	"testing"
)

const authJSONSample = `{"access_token":"tok-a","refresh_token":"tok-r","type":"codex","nested":{"k":"v"},"arr":["x","y"],"n":5,"b":true}`

// The standard library codec must be selected on 32-bit platforms only.
func TestAuthJSONUseStdlibOnlyOn32Bit(t *testing.T) {
	if want := bits.UintSize == 32; authJSONUseStdlib != want {
		t.Fatalf("authJSONUseStdlib = %v, want %v for %d-bit platform", authJSONUseStdlib, want, bits.UintSize)
	}
}

// Regression: moego/json blanked strings decoded into interface values on
// 32-bit platforms, which emptied every token of an auth file.
func TestUnmarshalAuthFileKeepsStringValues(t *testing.T) {
	var got map[string]any
	if err := UnmarshalAuthFile([]byte(authJSONSample), &got); err != nil {
		t.Fatalf("UnmarshalAuthFile() error = %v", err)
	}
	want := map[string]any{
		"access_token":  "tok-a",
		"refresh_token": "tok-r",
		"type":          "codex",
		"nested":        map[string]any{"k": "v"},
		"arr":           []any{"x", "y"},
		"n":             float64(5),
		"b":             true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded = %#v, want %#v", got, want)
	}
}

func TestMarshalAuthFileRoundTrip(t *testing.T) {
	in := map[string]any{"access_token": "tok-a", "type": "claude", "n": float64(2)}
	raw, err := MarshalAuthFile(in)
	if err != nil {
		t.Fatalf("MarshalAuthFile() error = %v", err)
	}
	var out map[string]any
	if err = UnmarshalAuthFile(raw, &out); err != nil {
		t.Fatalf("UnmarshalAuthFile() error = %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip = %#v, want %#v", out, in)
	}
}

type mergeSource struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Type         string `json:"type"`
}

// MergeMetadata is the common path that token storages use to build the
// auth file content; it must never drop the struct's string fields.
func TestMergeMetadataKeepsCredentialStrings(t *testing.T) {
	src := &mergeSource{AccessToken: "tok-a", RefreshToken: "tok-r", Type: "codex"}
	got, err := MergeMetadata(src, map[string]any{"email": "a@example.com"})
	if err != nil {
		t.Fatalf("MergeMetadata() error = %v", err)
	}
	want := map[string]any{
		"access_token":  "tok-a",
		"refresh_token": "tok-r",
		"type":          "codex",
		"email":         "a@example.com",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MergeMetadata() = %#v, want %#v", got, want)
	}
}
