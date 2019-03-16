// +build dev

package addons

import "net/http"

//go:generate vfsgendev -source="github.com/awesomenix/azk/assets/addons".Addons
// Addons contains project addons
var Addons http.FileSystem = http.Dir("../addons")
