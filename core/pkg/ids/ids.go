// Package ids generates prefixed ULIDs used as stable public identifiers
// across the Kave API, gRPC, and storage layers.
//
// Prefix registry:
// act, agt, aud, bge, bnd, cred, env, ivk, key, lim, mbr, nsp, org, pat, pol,
// prj, psn, role, rte, run, sec, ses, spn, tok, trc, use, usr.
package ids

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

// New returns a prefixed ULID, e.g. New("agt") -> "agt_01H...".
// An empty prefix yields a bare ULID.
func New(prefix string) string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)
	if prefix == "" {
		return id.String()
	}
	return fmt.Sprintf("%s_%s", prefix, id.String())
}
