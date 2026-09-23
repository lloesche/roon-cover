package display

import _ "embed"

// Unmodified Inter Regular 4.1, distributed under the SIL Open Font License.
//
//go:embed fonts/Inter-Regular.ttf
var interRegular []byte

// InterLicense includes the upstream copyright and complete font license.
//
//go:embed fonts/LICENSE.txt
var InterLicense string
