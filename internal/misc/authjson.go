package misc

import (
	// The standard library codec is imported on purpose; see the note below.
	stdjson "encoding/json"
	"math/bits"

	"github.com/MoeForge/moego/json"
)

// Auth file JSON codec.
//
// The project uses github.com/MoeForge/moego/json everywhere (see AGENTS.md).
// As of moego v0.1.0-alpha.5 (and current main), Unmarshal into interface
// values (map[string]any, any, map[string]string) returns EMPTY strings on
// 32-bit platforms such as Android armeabi-v7a. The cause is a hand-built
// string header in moego/json (intern.go keep and decany.go boxString) that
// stores the string length at byte offset 8, which is only right when a
// pointer is 8 bytes. Decoding into structs with string fields is not
// affected, and neither are numbers or booleans.
//
// Decoding an auth file into a map therefore silently blanked every token,
// and such files were then written back to disk with empty credentials.
//
// To avoid that, the helpers below are used ONLY where auth files are read
// or written. They use encoding/json when the platform is 32-bit and the
// regular moego/json everywhere else. The 32-bit check is a compile-time
// constant, so on 64-bit builds (amd64, arm64) the standard library branch is
// removed by the compiler and behavior is unchanged.
//
// Remove this file's 32-bit branch once moego fixes the bug upstream.

// authJSONUseStdlib reports whether auth file JSON must use encoding/json.
// It is true only on 32-bit platforms (uint is 32 bits wide).
const authJSONUseStdlib = bits.UintSize == 32

// MarshalAuthFile encodes v as JSON for an auth file.
func MarshalAuthFile(v any) ([]byte, error) {
	if authJSONUseStdlib {
		return stdjson.Marshal(v)
	}
	return json.Marshal(v)
}

// UnmarshalAuthFile decodes auth file JSON into v.
func UnmarshalAuthFile(data []byte, v any) error {
	if authJSONUseStdlib {
		return stdjson.Unmarshal(data, v)
	}
	return json.Unmarshal(data, v)
}
