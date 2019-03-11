// +build dev

package assets

import "net/http"

//go:generate vfsgendev -source="github.com/awesomenix/azk/assets".Assets
// Assets contains project assets.
var Assets http.FileSystem = http.Dir("../config")
