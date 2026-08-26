// Package site embeds the public UI comparison page in pqcaton-ui.
//
// ui-next.html remains an ordinary self-contained GitHub Pages file. Keeping
// the embedded bytes beside it makes the local UI server and the public site
// render the same prototype without maintaining a second copy.
package site

import _ "embed"

// UINext is the read-only workflow comparison prototype.
//
//go:embed ui-next.html
var UINext []byte
