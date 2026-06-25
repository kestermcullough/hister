// SPDX-License-Identifier: AGPL-3.0-or-later

package indexer

import (
	"errors"
	"github.com/asciimoo/hister/server/document"
)

// AddOrg renders TODO to HTML, stores it in d.HTML, and stores the raw
// source in d.Text for full-text indexing.
func AddOrg(d *document.Document, src []byte) error {
	return errors.New("Not implemented yet")
}
